package persistence_tests

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/event"
	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	"github.com/stretchr/testify/require"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"

	"inventory/internal/inventory/app"
	"inventory/internal/inventory/domain"
	eventfactory "inventory/internal/inventory/infra/event"
	inventorymysql "inventory/internal/inventory/infra/mysql"
	"inventory/internal/inventory/infra/mysql/migrations"
	"inventory/internal/inventory/infra/mysql/repository"
)

var client mysql.TransactionalClient
var testInventoryID = uuid.UUID{15: 1}
var upgradedProductID uuid.UUID
var upgradedReservationID uuid.UUID

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) (exitCode int) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	container, err := tcmysql.Run(ctx, "mysql:8.4",
		tcmysql.WithDatabase("inventory_test"),
		tcmysql.WithUsername("inventory"),
		tcmysql.WithPassword("inventory-test"),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() {
		if err := container.Terminate(context.Background()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			exitCode = 1
		}
	}()
	dsn, err := container.ConnectionString(ctx, "parseTime=true")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	connector := mysql.NewConnector()
	if err := connector.Open(dsn, mysql.Config{MaxConnections: 20}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() {
		if err := connector.Close(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			exitCode = 1
		}
	}()
	client = connector.TransactionalClient()
	runner, err := migrator.New(client, slog.Default())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	allMigrations := migrations.Database(client)
	if err := runner.Migrate(ctx, "inventory", allMigrations[:3]...); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	upgradedProductID = uuid.New()
	upgradedReservationID = uuid.New()
	fixtures := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO products (id, name, unit) VALUES (?, 'migration fixture', 'g')`, []any{upgradedProductID[:]}},
		{`INSERT INTO stocks (product_id, quantity, reserved_quantity) VALUES (?, 10, 4)`, []any{upgradedProductID[:]}},
		{`INSERT INTO reservations (id, status) VALUES (?, 'active')`, []any{upgradedReservationID[:]}},
		{
			`INSERT INTO reservation_items (reservation_id, product_id, quantity) VALUES (?, ?, 4)`,
			[]any{upgradedReservationID[:], upgradedProductID[:]},
		},
	}
	for _, fixture := range fixtures {
		if _, err := client.ExecContext(ctx, "test.migration_fixture", fixture.query, fixture.args...); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	if err := runner.Migrate(ctx, "inventory", allMigrations...); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return m.Run()
}

func TestMigrationPreservesReservation(t *testing.T) {
	svc := newService()
	stock, err := svc.GetStock(t.Context(), testInventoryID, upgradedProductID)
	require.NoError(t, err)
	require.EqualValues(t, 10, stock.Quantity)
	require.EqualValues(t, 4, stock.ReservedQuantity)
	require.NoError(t, svc.Consume(t.Context(), testInventoryID, upgradedReservationID))
	stock, err = svc.GetStock(t.Context(), testInventoryID, upgradedProductID)
	require.NoError(t, err)
	require.EqualValues(t, 6, stock.Quantity)
	require.Zero(t, stock.ReservedQuantity)
}

func newService() *app.Service {
	unit := uow.NewUnitOfWork(mysql.NewConnectionPool(client), inventorymysql.NewRepositoryProvider)
	dispatcher := event.DispatcherFunc(func(context.Context, event.Envelope) error { return nil })
	return app.NewService(unit, eventfactory.NewDispatcherFactory("inventory", dispatcher))
}

func addProduct(t *testing.T, svc *app.Service, quantity int64) uuid.UUID {
	t.Helper()
	product, err := svc.CreateProduct(t.Context(), "product", domain.UnitGrams)
	require.NoError(t, err)
	require.NoError(t, svc.AddStock(t.Context(), testInventoryID, product.ID(), quantity))
	return product.ID()
}

func TestReservationLifecycle(t *testing.T) {
	svc := newService()
	first := addProduct(t, svc, 10)
	second := addProduct(t, svc, 5)
	products := repository.NewProductRepository(t.Context(), client)
	product, err := products.GetByID(first)
	require.NoError(t, err)
	require.Equal(t, first, product.ID())
	allProducts, err := products.List(domain.ProductFilter{Limit: 20})
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(allProducts), 2)
	stocks := repository.NewStockRepository(t.Context(), client)
	allStocks, err := stocks.GetByProductIDs(testInventoryID, []uuid.UUID{first, second})
	require.NoError(t, err)
	require.Len(t, allStocks, 2)
	id := uuid.New()
	items := []domain.ReservationItem{
		{
			ProductID: first,
			Quantity:  7,
		},
		{
			ProductID: second,
			Quantity:  3,
		},
	}
	require.NoError(t, svc.Reserve(t.Context(), testInventoryID, id, items))
	require.NoError(t, svc.Reserve(t.Context(), testInventoryID, id, items))
	stock, err := stocks.GetByProductID(testInventoryID, first)
	require.NoError(t, err)
	require.EqualValues(t, 7, stock.ReservedQuantity)
	other := []domain.ReservationItem{
		{
			ProductID: first,
			Quantity:  1,
		},
	}
	require.ErrorIs(t, svc.Reserve(t.Context(), testInventoryID, id, other), domain.ErrReservationConflict)
	require.NoError(t, svc.Release(t.Context(), testInventoryID, id))
	require.NoError(t, svc.Release(t.Context(), testInventoryID, id))
	stock, err = stocks.GetByProductID(testInventoryID, first)
	require.NoError(t, err)
	require.EqualValues(t, 10, stock.Quantity)
	require.Zero(t, stock.ReservedQuantity)
	require.ErrorIs(t, svc.Reserve(t.Context(), testInventoryID, id, items), domain.ErrReservationCancelled)

	cancelled := uuid.New()
	require.NoError(t, svc.Release(t.Context(), testInventoryID, cancelled))
	require.ErrorIs(t, svc.Reserve(t.Context(), testInventoryID, cancelled, items), domain.ErrReservationCancelled)
	reservation, err := repository.NewReservationRepository(t.Context(), client).GetByID(testInventoryID, cancelled)
	require.NoError(t, err)
	require.Equal(t, domain.ReservationCancelled, reservation.Status())
}

func TestInsufficientRollsBackAll(t *testing.T) {
	svc := newService()
	first := addProduct(t, svc, 10)
	second := addProduct(t, svc, 10)
	if first.String() > second.String() {
		first, second = second, first
	}
	id := uuid.New()
	items := []domain.ReservationItem{
		{
			ProductID: first,
			Quantity:  5,
		},
		{
			ProductID: second,
			Quantity:  11,
		},
	}
	require.ErrorIs(t, svc.Reserve(t.Context(), testInventoryID, id, items), domain.ErrInsufficientStock)
	stocks := repository.NewStockRepository(t.Context(), client)
	for _, productID := range []uuid.UUID{first, second} {
		stock, err := stocks.GetByProductID(testInventoryID, productID)
		require.NoError(t, err)
		require.Zero(t, stock.ReservedQuantity)
	}
	reservation, err := repository.NewReservationRepository(t.Context(), client).GetByID(testInventoryID, id)
	require.NoError(t, err)
	require.Nil(t, reservation)
}

func TestConcurrentReleaseAndReserve(t *testing.T) {
	svc := newService()
	productID := addProduct(t, svc, 10)
	id := uuid.New()
	items := []domain.ReservationItem{
		{
			ProductID: productID,
			Quantity:  7,
		},
	}
	results := make(chan error, 2)
	go func() { results <- svc.Reserve(testOperationContext(), testInventoryID, id, items) }()
	go func() { results <- svc.Release(testOperationContext(), testInventoryID, id) }()
	for range 2 {
		err := <-results
		if err != nil {
			require.ErrorIs(t, err, domain.ErrReservationCancelled)
		}
	}
	stock, err := repository.NewStockRepository(t.Context(), client).GetByProductID(testInventoryID, productID)
	require.NoError(t, err)
	require.EqualValues(t, 10, stock.Quantity)
	require.Zero(t, stock.ReservedQuantity)
	reservation, err := repository.NewReservationRepository(t.Context(), client).GetByID(testInventoryID, id)
	require.NoError(t, err)
	require.Equal(t, domain.ReservationCancelled, reservation.Status())
}

func TestConcurrentReservations(t *testing.T) {
	for _, sameID := range []bool{false, true} {
		t.Run(fmt.Sprintf("same_id_%t", sameID), func(t *testing.T) {
			svc := newService()
			first := addProduct(t, svc, 10)
			second := addProduct(t, svc, 10)
			ids := []uuid.UUID{uuid.New(), uuid.New()}
			if sameID {
				ids[1] = ids[0]
			}
			results := make(chan error, 2)
			var group sync.WaitGroup
			for index := range 2 {
				group.Add(1)
				go func() {
					defer group.Done()
					items := []domain.ReservationItem{
						{
							ProductID: first,
							Quantity:  7,
						},
						{
							ProductID: second,
							Quantity:  7,
						},
					}
					if index == 1 {
						items[0], items[1] = items[1], items[0]
					}
					results <- svc.Reserve(testOperationContext(), testInventoryID, ids[index], items)
				}()
			}
			group.Wait()
			close(results)
			successes, shortages := 0, 0
			for err := range results {
				switch {
				case err == nil:
					successes++
				case errors.Is(err, domain.ErrInsufficientStock):
					shortages++
				default:
					require.NoError(t, err)
				}
			}
			if sameID {
				require.Equal(t, 2, successes)
			} else {
				require.Equal(t, 1, successes)
				require.Equal(t, 1, shortages)
			}
			stocks := repository.NewStockRepository(t.Context(), client)
			for _, productID := range []uuid.UUID{first, second} {
				stock, err := stocks.GetByProductID(testInventoryID, productID)
				require.NoError(t, err)
				require.EqualValues(t, 7, stock.ReservedQuantity)
				require.EqualValues(t, 3, stock.Available())
			}
		})
	}
}

type operationContextKey struct{}

func testOperationContext() context.Context {
	return context.WithValue(context.Background(), operationContextKey{}, uuid.New())
}

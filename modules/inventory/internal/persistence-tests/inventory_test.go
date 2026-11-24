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

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	"github.com/stretchr/testify/require"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"

	"inventory/internal/inventory/app"
	"inventory/internal/inventory/domain"
	inventorymysql "inventory/internal/inventory/infra/mysql"
	"inventory/internal/inventory/infra/mysql/migrations"
	"inventory/internal/inventory/infra/mysql/repository"
)

var client mysql.TransactionalClient

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
	if err := runner.Migrate(ctx, "inventory", migrations.Database(client)...); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return m.Run()
}

func newService() *app.Service {
	unit := uow.NewUnitOfWork(mysql.NewConnectionPool(client), inventorymysql.NewRepositoryProvider)
	return app.NewService(unit, nil)
}

func addProduct(t *testing.T, svc *app.Service, quantity int64) uuid.UUID {
	t.Helper()
	id := uuid.New()
	product, err := domain.NewProduct(id, "product", domain.UnitGrams)
	require.NoError(t, err)
	require.NoError(t, svc.CreateProduct(t.Context(), product))
	require.NoError(t, svc.AddStock(t.Context(), id, quantity))
	return id
}

func TestReservationLifecycle(t *testing.T) {
	svc := newService()
	first := addProduct(t, svc, 10)
	second := addProduct(t, svc, 5)
	products := repository.NewProductRepository(t.Context(), client)
	product, err := products.GetByID(first)
	require.NoError(t, err)
	require.Equal(t, first, product.ID)
	allProducts, err := products.List()
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(allProducts), 2)
	stocks := repository.NewStockRepository(t.Context(), client)
	allStocks, err := stocks.GetByProductIDs([]uuid.UUID{first, second})
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
	require.NoError(t, svc.Reserve(t.Context(), id, items))
	require.NoError(t, svc.Reserve(t.Context(), id, items))
	stock, err := stocks.GetByProductID(first)
	require.NoError(t, err)
	require.EqualValues(t, 7, stock.ReservedQuantity)
	other := []domain.ReservationItem{
		{
			ProductID: first,
			Quantity:  1,
		},
	}
	require.ErrorIs(t, svc.Reserve(t.Context(), id, other), domain.ErrReservationConflict)
	require.NoError(t, svc.Release(t.Context(), id))
	require.NoError(t, svc.Release(t.Context(), id))
	stock, err = stocks.GetByProductID(first)
	require.NoError(t, err)
	require.EqualValues(t, 10, stock.Quantity)
	require.Zero(t, stock.ReservedQuantity)
	require.ErrorIs(t, svc.Reserve(t.Context(), id, items), domain.ErrReservationCancelled)

	cancelled := uuid.New()
	require.NoError(t, svc.Release(t.Context(), cancelled))
	require.ErrorIs(t, svc.Reserve(t.Context(), cancelled, items), domain.ErrReservationCancelled)
	reservation, err := repository.NewReservationRepository(t.Context(), client).GetByID(cancelled)
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
	require.ErrorIs(t, svc.Reserve(t.Context(), id, items), domain.ErrInsufficientStock)
	stocks := repository.NewStockRepository(t.Context(), client)
	for _, productID := range []uuid.UUID{first, second} {
		stock, err := stocks.GetByProductID(productID)
		require.NoError(t, err)
		require.Zero(t, stock.ReservedQuantity)
	}
	reservation, err := repository.NewReservationRepository(t.Context(), client).GetByID(id)
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
	go func() { results <- svc.Reserve(context.Background(), id, items) }()
	go func() { results <- svc.Release(context.Background(), id) }()
	for range 2 {
		err := <-results
		if err != nil {
			require.ErrorIs(t, err, domain.ErrReservationCancelled)
		}
	}
	stock, err := repository.NewStockRepository(t.Context(), client).GetByProductID(productID)
	require.NoError(t, err)
	require.EqualValues(t, 10, stock.Quantity)
	require.Zero(t, stock.ReservedQuantity)
	reservation, err := repository.NewReservationRepository(t.Context(), client).GetByID(id)
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
					results <- svc.Reserve(context.Background(), ids[index], items)
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
				stock, err := stocks.GetByProductID(productID)
				require.NoError(t, err)
				require.EqualValues(t, 7, stock.ReservedQuantity)
				require.EqualValues(t, 3, stock.Available())
			}
		})
	}
}

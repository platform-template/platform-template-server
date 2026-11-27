//go:build integration

package migrations_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	"github.com/stretchr/testify/require"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	internalapi "inventory/api/server/internalapi"
	"inventory/internal/inventory/app"
	"inventory/internal/inventory/domain"
	internalhandler "inventory/internal/inventory/infra/handlers/internalapi"
	inventorymysql "inventory/internal/inventory/infra/mysql"
	"inventory/internal/inventory/infra/mysql/migrations"
)

func TestMultipleInventoryMigrationAndReservations(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	container, err := tcmysql.Run(ctx, "mysql:8.4", tcmysql.WithDatabase("inventory_test"),
		tcmysql.WithUsername("inventory"), tcmysql.WithPassword("inventory-test"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(context.Background())) })
	dsn, err := container.ConnectionString(ctx, "parseTime=true")
	require.NoError(t, err)
	connector := mysql.NewConnector()
	require.NoError(t, connector.Open(dsn, mysql.Config{MaxConnections: 10}))
	t.Cleanup(func() { require.NoError(t, connector.Close()) })
	client := connector.TransactionalClient()
	runner, err := migrator.New(client, slog.Default())
	require.NoError(t, err)
	all := migrations.Database(client)
	require.NoError(t, runner.Migrate(ctx, "inventory", all[:len(all)-1]...))
	productID, reservationID := uuid.New(), uuid.New()
	_, err = client.ExecContext(ctx, "fixture.product", `INSERT INTO products VALUES (?, 'греча', 'g')`, productID[:])
	require.NoError(t, err)
	_, err = client.ExecContext(ctx, "fixture.stock", `INSERT INTO stocks VALUES (?, 10, 4)`, productID[:])
	require.NoError(t, err)
	_, err = client.ExecContext(
		ctx,
		"fixture.reservation",
		`INSERT INTO reservations VALUES (?, 'active')`,
		reservationID[:],
	)
	require.NoError(t, err)
	_, err = client.ExecContext(
		ctx,
		"fixture.item",
		`INSERT INTO reservation_items VALUES (?, ?, 4)`,
		reservationID[:],
		productID[:],
	)
	require.NoError(t, err)
	require.NoError(t, runner.Migrate(ctx, "inventory", all...))
	// MySQL DDL is not transactional; an interrupted migration must be safe to retry.
	require.NoError(t, all[len(all)-1].Up(ctx))

	legacyID := uuid.UUID{15: 1}
	otherID := uuid.New()
	unit := uow.NewUnitOfWork(mysql.NewConnectionPool(client), inventorymysql.NewRepositoryProvider)
	service := app.NewService(unit, nil)
	execute := func(fn func(*inventorymysql.RepositoryProvider, context.Context) error) error {
		opCtx, done := context.WithCancel(ctx)
		defer done()
		return unit.ExecuteWithRepositoryProvider(
			opCtx,
			func(p *inventorymysql.RepositoryProvider) error { return fn(p, opCtx) },
		)
	}
	require.NoError(t, execute(func(p *inventorymysql.RepositoryProvider, c context.Context) error {
		inventory, err := domain.NewInventory(otherID, "Морозилка")
		if err != nil {
			return err
		}
		if err = p.Inventory(c).Create(inventory); err != nil {
			return err
		}
		return p.Stock(c).Add(otherID, productID, 20)
	}))
	require.NoError(t, execute(func(p *inventorymysql.RepositoryProvider, c context.Context) error {
		legacy, err := p.Stock(c).GetByProductID(legacyID, productID)
		require.NoError(t, err)
		require.EqualValues(t, 10, legacy.Quantity)
		require.EqualValues(t, 4, legacy.ReservedQuantity)
		other, err := p.Stock(c).GetByProductID(otherID, productID)
		require.NoError(t, err)
		require.EqualValues(t, 20, other.Quantity)
		require.ErrorIs(t, p.Reservation(c).Consume(otherID, reservationID), domain.ErrReservationConflict)
		return p.Reservation(c).Consume(legacyID, reservationID)
	}))
	require.NoError(t, execute(func(p *inventorymysql.RepositoryProvider, c context.Context) error {
		legacy, err := p.Stock(c).GetByProductID(legacyID, productID)
		require.NoError(t, err)
		require.EqualValues(t, 6, legacy.Quantity)
		require.Zero(t, legacy.ReservedQuantity)
		other, err := p.Stock(c).GetByProductID(otherID, productID)
		require.NoError(t, err)
		require.EqualValues(t, 20, other.Quantity)
		return p.Reservation(c).Consume(legacyID, reservationID)
	}))
	require.NoError(t, execute(func(p *inventorymysql.RepositoryProvider, c context.Context) error {
		product, err := domain.NewProduct(uuid.New(), "молоко", domain.UnitMillilitres)
		if err != nil {
			return err
		}
		if err := p.Product(c).Create(&product); err != nil {
			return err
		}
		stock, err := p.Stock(c).GetByProductID(otherID, product.ID())
		require.NoError(t, err)
		require.Nil(t, stock)
		if err := p.Stock(c).Add(otherID, product.ID(), 8); err != nil {
			return err
		}
		id := uuid.New()
		reservation, err := domain.NewReservation(
			id,
			otherID,
			[]domain.ReservationItem{{ProductID: product.ID(), Quantity: 6}},
		)
		if err != nil {
			return err
		}
		if err := p.Reservation(c).Reserve(reservation); err != nil {
			return err
		}
		require.NoError(t, p.Reservation(c).Reserve(reservation))
		require.ErrorIs(t, p.Stock(c).Remove(otherID, product.ID(), 3), domain.ErrInsufficientStock)
		conflict, err := domain.NewReservation(
			id,
			otherID,
			[]domain.ReservationItem{{ProductID: product.ID(), Quantity: 5}},
		)
		require.NoError(t, err)
		require.ErrorIs(t, p.Reservation(c).Reserve(conflict), domain.ErrReservationConflict)
		if err := p.Reservation(c).Release(otherID, id); err != nil {
			return err
		}
		require.ErrorIs(t, p.Reservation(c).Reserve(conflict), domain.ErrReservationConflict)
		require.ErrorIs(t, p.Reservation(c).Reserve(reservation), domain.ErrReservationCancelled)
		markerID := uuid.New()
		if err := p.Reservation(c).Release(otherID, markerID); err != nil {
			return err
		}
		marker, err := domain.NewReservation(
			markerID,
			otherID,
			[]domain.ReservationItem{{ProductID: product.ID(), Quantity: 1}},
		)
		require.NoError(t, err)
		require.ErrorIs(t, p.Reservation(c).Reserve(marker), domain.ErrReservationCancelled)
		return nil
	}))
	missingID, failedReservationID := productID, uuid.New()
	for i := len(missingID) - 1; i >= 0; i-- {
		missingID[i]++
		if missingID[i] != 0 {
			break
		}
	}
	require.NoError(t, execute(func(p *inventorymysql.RepositoryProvider, c context.Context) error {
		product, err := domain.NewProduct(missingID, "кефир", domain.UnitMillilitres)
		if err != nil {
			return err
		}
		return p.Product(c).Create(&product)
	}))
	require.ErrorIs(t, execute(func(p *inventorymysql.RepositoryProvider, c context.Context) error {
		reservation, err := domain.NewReservation(failedReservationID, otherID, []domain.ReservationItem{
			{ProductID: productID, Quantity: 1},
			{ProductID: missingID, Quantity: 1},
		})
		if err != nil {
			return err
		}
		return p.Reservation(c).Reserve(reservation)
	}), domain.ErrInsufficientStock)
	require.NoError(t, execute(func(p *inventorymysql.RepositoryProvider, c context.Context) error {
		stock, err := p.Stock(c).GetByProductID(otherID, productID)
		require.NoError(t, err)
		require.Zero(t, stock.ReservedQuantity)
		reservation, err := p.Reservation(c).GetByID(otherID, failedReservationID)
		require.NoError(t, err)
		require.Nil(t, reservation)
		return nil
	}))
	rollback := errors.New("roll back outer transaction")
	opCtx, done := context.WithCancel(ctx)
	err = unit.ExecuteWithRepositoryProvider(opCtx, func(_ *inventorymysql.RepositoryProvider) error {
		if err := service.AddStock(opCtx, otherID, productID, 7); err != nil {
			return err
		}
		return rollback
	})
	done()
	require.ErrorIs(t, err, rollback)
	require.NoError(t, execute(func(p *inventorymysql.RepositoryProvider, c context.Context) error {
		stock, err := p.Stock(c).GetByProductID(otherID, productID)
		require.NoError(t, err)
		require.EqualValues(t, 20, stock.Quantity)
		return nil
	}))
	newInventory, err := service.CreateInventory(ctx, "Шкаф")
	require.NoError(t, err)
	require.NoError(t, service.RenameInventory(ctx, newInventory.ID(), "Буфет"))
	loaded, err := service.GetInventory(ctx, newInventory.ID())
	require.NoError(t, err)
	require.Equal(t, "Буфет", loaded.Name())
	stock, err := service.GetStock(ctx, newInventory.ID(), productID)
	require.NoError(t, err)
	require.Nil(t, stock)
	require.NoError(t, service.CreateStock(ctx, newInventory.ID(), productID))
	require.ErrorIs(t, service.CreateStock(ctx, newInventory.ID(), productID), domain.ErrStockExists)
	stock, err = service.GetStock(ctx, newInventory.ID(), productID)
	require.NoError(t, err)
	require.NotNil(t, stock)
	require.Zero(t, stock.Quantity)
	require.NoError(t, service.AddStock(ctx, newInventory.ID(), productID, 5))
	require.NoError(t, service.RemoveStock(ctx, newInventory.ID(), productID, 2))
	stock, err = service.GetStock(ctx, newInventory.ID(), productID)
	require.NoError(t, err)
	require.EqualValues(t, 3, stock.Quantity)
	grpc := internalhandler.NewHandler(service, slog.Default())
	response, err := grpc.GetStock(ctx, &internalapi.GetStockRequest{
		InventoryId: newInventory.ID().String(), ProductId: productID.String(),
	})
	require.NoError(t, err)
	require.True(t, response.Exists)
	require.EqualValues(t, 3, response.Available)
	reservation := &internalapi.ReserveRequest{
		InventoryId: newInventory.ID().String(), ReservationId: uuid.New().String(),
		Items: []*internalapi.ReservationItem{{ProductId: productID.String(), Quantity: 2}},
	}
	_, err = grpc.Reserve(ctx, reservation)
	require.NoError(t, err)
	_, err = grpc.Reserve(ctx, reservation)
	require.NoError(t, err)
	_, err = grpc.Consume(ctx, &internalapi.ReservationRequest{
		InventoryId: newInventory.ID().String(), ReservationId: reservation.ReservationId,
	})
	require.NoError(t, err)
	_, err = grpc.Reserve(ctx, reservation)
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	_, err = grpc.GetStock(ctx, &internalapi.GetStockRequest{InventoryId: "invalid", ProductId: productID.String()})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = grpc.Release(ctx, &internalapi.ReservationRequest{
		InventoryId: uuid.New().String(), ReservationId: uuid.New().String(),
	})
	require.Equal(t, codes.NotFound, status.Code(err))
}

func TestCleanMigrationDoesNotCreateDefaultInventory(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	container, err := tcmysql.Run(ctx, "mysql:8.4", tcmysql.WithDatabase("inventory_test"),
		tcmysql.WithUsername("inventory"), tcmysql.WithPassword("inventory-test"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(context.Background())) })
	dsn, err := container.ConnectionString(ctx, "parseTime=true")
	require.NoError(t, err)
	connector := mysql.NewConnector()
	require.NoError(t, connector.Open(dsn, mysql.Config{MaxConnections: 10}))
	t.Cleanup(func() { require.NoError(t, connector.Close()) })
	client := connector.TransactionalClient()
	runner, err := migrator.New(client, slog.Default())
	require.NoError(t, err)
	require.NoError(t, runner.Migrate(ctx, "inventory", migrations.Database(client)...))
	var count int
	require.NoError(t, client.QueryRowContext(ctx, "inventory.count", `SELECT COUNT(*) FROM inventories`).Scan(&count))
	require.Zero(t, count)
}

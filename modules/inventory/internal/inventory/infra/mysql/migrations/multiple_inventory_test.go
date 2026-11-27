//go:build integration

package migrations_test

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/event"
	"github.com/distributed-programming-2026/go-sdk/pkg/event/outbox"
	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	domainevent "github.com/distributed-programming-2026/lib/event"

	authzapi "authz/api/server/internalapi"
	inventoryevents "inventory/api/events"
	internalapi "inventory/api/server/internalapi"
	"inventory/internal/inventory/app"
	"inventory/internal/inventory/domain"
	eventfactory "inventory/internal/inventory/infra/event"
	internalhandler "inventory/internal/inventory/infra/handlers/internalapi"
	publichandler "inventory/internal/inventory/infra/handlers/public"
	inventorymysql "inventory/internal/inventory/infra/mysql"
	"inventory/internal/inventory/infra/mysql/migrations"
	sessionapi "session/api/server/internalapi"
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
	require.NoError(t, runner.Migrate(ctx, "inventory", all[:3]...))
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
	require.NoError(t, runner.Migrate(ctx, "event-outbox", outbox.Migrations(client)...))
	// MySQL DDL is not transactional; an interrupted migration must be safe to retry.
	require.NoError(t, all[3].Up(ctx))

	legacyID := uuid.UUID{15: 1}
	otherID := uuid.New()
	unit := uow.NewUnitOfWork(mysql.NewConnectionPool(client), inventorymysql.NewRepositoryProvider)
	transport := event.DispatcherFunc(func(context.Context, event.Envelope) error { return nil })
	dispatcher, _, err := outbox.Decorate(transport, uow.NewLockableUnitOfWork(unit), outbox.Config{Transport: "test"})
	require.NoError(t, err)
	service := app.NewService(unit, eventfactory.NewDispatcherFactory("inventory", dispatcher))
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
	grpcHandler := internalhandler.NewHandler(service, slog.Default())
	response, err := grpcHandler.GetStock(ctx, &internalapi.GetStockRequest{
		InventoryId: newInventory.ID().String(), ProductId: productID.String(),
	})
	require.NoError(t, err)
	require.True(t, response.Exists)
	require.EqualValues(t, 3, response.Available)
	reservation := &internalapi.ReserveRequest{
		InventoryId: newInventory.ID().String(), ReservationId: uuid.New().String(),
		Items: []*internalapi.ReservationItem{{ProductId: productID.String(), Quantity: 2}},
	}
	_, err = grpcHandler.Reserve(ctx, reservation)
	require.NoError(t, err)
	_, err = grpcHandler.Reserve(ctx, reservation)
	require.NoError(t, err)
	_, err = grpcHandler.Consume(ctx, &internalapi.ReservationRequest{
		InventoryId: newInventory.ID().String(), ReservationId: reservation.ReservationId,
	})
	require.NoError(t, err)
	_, err = grpcHandler.Reserve(ctx, reservation)
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
	_, err = grpcHandler.GetStock(
		ctx,
		&internalapi.GetStockRequest{InventoryId: "invalid", ProductId: productID.String()},
	)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = grpcHandler.Release(ctx, &internalapi.ReservationRequest{
		InventoryId: uuid.New().String(), ReservationId: uuid.New().String(),
	})
	require.Equal(t, codes.NotFound, status.Code(err))
	ownerID, strangerID := uuid.New(), uuid.New()
	owned, err := service.CreateOwnedInventory(ctx, ownerID, "Холодильник")
	require.NoError(t, err)
	var payload []byte
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.last_event", `SELECT payload FROM event_outbox ORDER BY id DESC LIMIT 1`).
			Scan(&payload),
	)
	envelope, err := event.Unmarshal(payload)
	require.NoError(t, err)
	require.Equal(t, "InventoryCreated", envelope.Type())
	var created inventoryevents.InventoryCreated
	require.NoError(t, envelope.UnmarshalPayload(&created))
	require.Equal(t, owned.ID().String(), created.InventoryId)
	require.Equal(t, ownerID.String(), created.ActorId)
	isOwner, err := service.IsInventoryOwner(ctx, ownerID, owned.ID())
	require.NoError(t, err)
	require.True(t, isOwner)
	isOwner, err = service.IsInventoryOwner(ctx, strangerID, owned.ID())
	require.NoError(t, err)
	require.False(t, isOwner)
	var inventoryCountBefore, inventoryCountAfter int
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.inventory.count", `SELECT COUNT(*) FROM inventories`).
			Scan(&inventoryCountBefore),
	)
	dispatchFailure := errors.New("outbox failure")
	failingService := app.NewService(unit, failingDispatcherFactory{err: dispatchFailure})
	_, err = failingService.CreateOwnedInventory(ctx, ownerID, "Без события")
	require.ErrorIs(t, err, dispatchFailure)
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.inventory.count", `SELECT COUNT(*) FROM inventories`).
			Scan(&inventoryCountAfter),
	)
	require.Equal(t, inventoryCountBefore, inventoryCountAfter)
	isOwner, err = service.IsInventoryOwner(ctx, ownerID, legacyID)
	require.NoError(t, err)
	require.False(t, isOwner)
	inventories, err := service.ListOwnedInventories(ctx, ownerID)
	require.NoError(t, err)
	require.Len(t, inventories, 1)
	require.Equal(t, owned.ID(), inventories[0].ID())
	inventories, err = service.ListOwnedInventories(ctx, strangerID)
	require.NoError(t, err)
	require.Empty(t, inventories)
	var rolledBackID uuid.UUID
	var eventsBefore, eventsAfter int
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.count_outbox", `SELECT COUNT(*) FROM event_outbox`).Scan(&eventsBefore),
	)
	opCtx, done = context.WithCancel(ctx)
	err = unit.ExecuteWithRepositoryProvider(opCtx, func(_ *inventorymysql.RepositoryProvider) error {
		inventory, err := service.CreateOwnedInventory(opCtx, ownerID, "Не создан")
		if err != nil {
			return err
		}
		rolledBackID = inventory.ID()
		return rollback
	})
	done()
	require.ErrorIs(t, err, rollback)
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.count_outbox", `SELECT COUNT(*) FROM event_outbox`).Scan(&eventsAfter),
	)
	require.Equal(t, eventsBefore, eventsAfter)
	_, err = service.GetInventory(ctx, rolledBackID)
	require.ErrorIs(t, err, domain.ErrInventoryNotFound)
	isOwner, err = service.IsInventoryOwner(ctx, ownerID, rolledBackID)
	require.NoError(t, err)
	require.False(t, isOwner)

	testPublicHTTP(t, service, client, ownerID, strangerID, legacyID, owned.ID(), productID)
}

type failingDispatcherFactory struct{ err error }

func (f failingDispatcherFactory) NewDispatcher(context.Context) domainevent.Dispatcher {
	return failingDispatcher(f)
}

type failingDispatcher struct{ err error }

func (d failingDispatcher) Dispatch(proto.Message) error { return d.err }

func testPublicHTTP(
	t *testing.T,
	service *app.Service,
	client mysql.TransactionalClient,
	ownerID, strangerID, legacyID, ownedID, productID uuid.UUID,
) {
	t.Helper()
	router := chi.NewRouter()
	register, err := publichandler.Register(service, slog.Default(), testSessions{users: map[string]uuid.UUID{
		"owner": ownerID, "stranger": strangerID,
	}}, testAuthz{})
	require.NoError(t, err)
	require.NoError(t, register(router))
	request := func(method, path, token, body, origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			req.AddCookie(
				&http.Cookie{
					Name:     "session",
					Value:    token,
					Secure:   true,
					HttpOnly: true,
					SameSite: http.SameSiteStrictMode,
				},
			)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		req.Header.Set("X-User-ID", ownerID.String())
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		return response
	}
	base := "/api/public/v1/inventories/" + ownedID.String()
	created := request("POST", "/api/public/v1/inventories", "owner", `{"name":"Морозилка"}`, "")
	require.Equal(t, http.StatusCreated, created.Code)
	var createdInventory struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &createdInventory))
	require.NotEmpty(t, createdInventory.ID)
	require.Equal(
		t,
		http.StatusNotFound,
		request("GET", "/api/public/v1/inventories/"+createdInventory.ID, "stranger", "", "").Code,
	)
	list := request("GET", "/api/public/v1/inventories", "stranger", "", "")
	require.Equal(t, http.StatusOK, list.Code)
	require.NotContains(t, list.Body.String(), createdInventory.ID)
	require.Equal(t, http.StatusUnauthorized, request("GET", base, "", "", "").Code)
	require.Equal(t, http.StatusNotFound, request("GET", base, "stranger", "", "").Code)
	require.Equal(
		t,
		http.StatusNotFound,
		request("GET", "/api/public/v1/inventories/"+legacyID.String(), "owner", "", "").Code,
	)
	require.Equal(t, http.StatusOK, request("GET", base, "owner", "", "").Code)
	stockPath := base + "/stocks/" + productID.String()
	httpResponse := request("GET", stockPath, "owner", "", "")
	require.Equal(t, http.StatusOK, httpResponse.Code)
	require.Contains(t, httpResponse.Body.String(), `"exists":false`)
	require.Equal(
		t,
		http.StatusForbidden,
		request("POST", stockPath+"/add", "owner", `{"quantity":5}`, "https://example.org").Code,
	)
	require.Equal(t, http.StatusNotFound, request("POST", stockPath+"/add", "stranger", `{"quantity":5}`, "").Code)
	require.Equal(t, http.StatusNoContent, request("POST", stockPath+"/add", "owner", `{"quantity":5}`, "").Code)
	var payload []byte
	require.NoError(
		t,
		client.QueryRowContext(t.Context(), "test.last_event", `SELECT payload FROM event_outbox ORDER BY id DESC LIMIT 1`).
			Scan(&payload),
	)
	envelope, err := event.Unmarshal(payload)
	require.NoError(t, err)
	var adjusted inventoryevents.StockAdjusted
	require.NoError(t, envelope.UnmarshalPayload(&adjusted))
	require.Equal(t, ownerID.String(), adjusted.ActorId)
	require.EqualValues(t, 5, adjusted.QuantityDelta)
	httpResponse = request("GET", stockPath, "owner", "", "")
	require.Equal(t, http.StatusOK, httpResponse.Code)
	require.Contains(t, httpResponse.Body.String(), `"quantity":5`)
	require.Equal(
		t,
		http.StatusForbidden,
		request("POST", "/api/public/v1/products", "owner", `{"name":"чай","unit":"g"}`, "").Code,
	)
}

type testSessions struct {
	sessionapi.SessionServiceClient
	users map[string]uuid.UUID
}

func (s testSessions) ResolveSession(
	_ context.Context,
	req *sessionapi.ResolveSessionRequest,
	_ ...grpc.CallOption,
) (*sessionapi.ResolveSessionResponse, error) {
	id, ok := s.users[req.GetToken()]
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "invalid session")
	}
	return &sessionapi.ResolveSessionResponse{UserId: id.String()}, nil
}

type testAuthz struct{ authzapi.AuthzServiceClient }

func (testAuthz) IsSuperuser(
	context.Context,
	*authzapi.IsSuperuserRequest,
	...grpc.CallOption,
) (*authzapi.IsSuperuserResponse, error) {
	return &authzapi.IsSuperuserResponse{IsSuperuser: false}, nil
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

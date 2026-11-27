//go:build integration

package migrations_test

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"uuid"

	inventoryevents "inventory/api/events"
	"inventory/internal/inventory/app"
	"inventory/internal/inventory/domain"
	handler "inventory/internal/inventory/infra/handlers/events"
	inventorymysql "inventory/internal/inventory/infra/mysql"
	mealplannerevents "mealplanner/api/events"

	"github.com/distributed-programming-2026/go-sdk/pkg/event"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	"github.com/stretchr/testify/require"
)

func testMealInbox(
	t *testing.T,
	service *app.Service,
	unit uow.UnitOfWorkWithRepositoryProvider[*inventorymysql.RepositoryProvider],
	client mysql.TransactionalClient,
) {
	t.Helper()
	ctx := t.Context()
	owner := uuid.New()
	inventory, err := service.CreateOwnedInventory(ctx, owner, "Meal inventory")
	require.NoError(t, err)
	product, err := service.CreateProduct(ctx, "Meal allocation", domain.Unit("g"))
	require.NoError(t, err)
	require.NoError(t, service.AddStock(ctx, inventory.ID(), product.ID(), 3))
	request := &mealplannerevents.MealPlanningRequested{
		PlanId:         uuid.New().String(),
		ItemId:         uuid.New().String(),
		Revision:       uuid.New().String(),
		ActorId:        owner.String(),
		ShoppingListId: uuid.New().String(),
		InventoryIds:   []string{inventory.ID().String()},
		Ingredients:    []*mealplannerevents.MealIngredient{{ProductId: product.ID().String(), Quantity: 5}},
	}
	envelope, err := event.New(ctx, "mealplanner", "MealPlanningRequested", request)
	require.NoError(t, err)
	base := handler.NewHandler(slog.Default(), service)
	failure := errors.New("fail after allocation")
	failing := event.HandlerFunc(func(ctx context.Context, envelope event.Envelope) error {
		if err := base.Handle(ctx, envelope); err != nil {
			return err
		}
		return failure
	})
	consumer := "meal-test-" + uuid.New().String()
	wrapped, err := handler.WithInbox(failing, unit, consumer)
	require.NoError(t, err)
	var before, after int
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.outbox.count", `SELECT COUNT(*) FROM event_outbox`).Scan(&before),
	)
	require.ErrorIs(t, wrapped.Handle(ctx, envelope), failure)
	stock, err := service.GetStock(ctx, inventory.ID(), product.ID())
	require.NoError(t, err)
	require.Zero(t, stock.ReservedQuantity)
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.outbox.count", `SELECT COUNT(*) FROM event_outbox`).Scan(&after),
	)
	require.Equal(t, before, after)
	var count int
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.inbox.count", `SELECT COUNT(*) FROM event_inbox WHERE consumer = ? AND event_id = ?`, consumer, envelope.ID()).
			Scan(&count),
	)
	require.Zero(t, count)
	wrapped, err = handler.WithInbox(base, unit, consumer)
	require.NoError(t, err)
	require.NoError(t, wrapped.Handle(ctx, envelope))
	require.NoError(t, wrapped.Handle(ctx, envelope))
	stock, err = service.GetStock(ctx, inventory.ID(), product.ID())
	require.NoError(t, err)
	require.EqualValues(t, 3, stock.ReservedQuantity)
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.outbox.count", `SELECT COUNT(*) FROM event_outbox`).Scan(&after),
	)
	require.Equal(t, before+2, after)
	var payload []byte
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.allocation.event", `SELECT payload FROM event_outbox ORDER BY id DESC LIMIT 1 OFFSET 1`).
			Scan(&payload),
	)
	result, err := event.Unmarshal(payload)
	require.NoError(t, err)
	require.Equal(t, "AllocationCompleted", result.Type())
	var allocation inventoryevents.AllocationCompleted
	require.NoError(t, result.UnmarshalPayload(&allocation))
	require.Equal(t, request.Revision, allocation.Revision)
	require.Len(t, allocation.Shortages, 1)
	require.EqualValues(t, 2, allocation.Shortages[0].Quantity)
	identity := domain.AllocationIdentity{
		Revision:       uuid.MustParse(request.Revision),
		ActorID:        owner,
		PlanID:         uuid.MustParse(request.PlanId),
		ItemID:         uuid.MustParse(request.ItemId),
		ShoppingListID: uuid.MustParse(request.ShoppingListId),
		InventoryIDs:   []uuid.UUID{inventory.ID()},
	}
	other := uuid.New()
	foreign, err := service.CreateOwnedInventory(ctx, other, "Foreign inventory")
	require.NoError(t, err)
	forged := identity
	forged.ActorID = other
	forged.InventoryIDs = []uuid.UUID{foreign.ID()}
	require.ErrorIs(t, service.CancelMealAllocation(ctx, forged), domain.ErrAllocationConflict)
	unchanged, err := service.GetStock(ctx, inventory.ID(), product.ID())
	require.NoError(t, err)
	require.EqualValues(t, 3, unchanged.ReservedQuantity)
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.outbox.count", `SELECT COUNT(*) FROM event_outbox`).Scan(&before),
	)
	cancelCtx, done := context.WithCancel(ctx)
	err = unit.ExecuteWithRepositoryProvider(cancelCtx, func(_ *inventorymysql.RepositoryProvider) error {
		if err := service.CancelMealAllocation(cancelCtx, identity); err != nil {
			return err
		}
		return failure
	})
	done()
	require.ErrorIs(t, err, failure)
	unchanged, err = service.GetStock(ctx, inventory.ID(), product.ID())
	require.NoError(t, err)
	require.EqualValues(t, 3, unchanged.ReservedQuantity)
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.outbox.count", `SELECT COUNT(*) FROM event_outbox`).Scan(&after),
	)
	require.Equal(t, before, after)
	require.NoError(t, service.CancelMealAllocation(ctx, identity))
	require.NoError(t, service.CancelMealAllocation(ctx, identity))
	released, err := service.GetStock(ctx, inventory.ID(), product.ID())
	require.NoError(t, err)
	require.Zero(t, released.ReservedQuantity)
	early := identity
	early.Revision = uuid.New()
	require.NoError(t, service.CancelMealAllocation(ctx, early))
	late := app.MealAllocation{
		PlanID:         early.PlanID,
		ItemID:         early.ItemID,
		Revision:       early.Revision,
		ActorID:        early.ActorID,
		ShoppingListID: early.ShoppingListID,
		InventoryIDs:   early.InventoryIDs,
		Items:          []domain.ReservationItem{{ProductID: product.ID(), Quantity: 1}},
	}
	require.NoError(t, service.AllocateMeal(ctx, late))
	released, err = service.GetStock(ctx, inventory.ID(), product.ID())
	require.NoError(t, err)
	require.Zero(t, released.ReservedQuantity)

	legacy := identity
	legacy.Revision = uuid.New()
	legacyCtx, done := context.WithCancel(ctx)
	err = unit.ExecuteWithRepositoryProvider(legacyCtx, func(p *inventorymysql.RepositoryProvider) error {
		_, _, err := p.Allocation(legacyCtx).
			Allocate(legacy.Revision, legacy.InventoryIDs, []domain.ReservationItem{{ProductID: product.ID(), Quantity: 1}})
		return err
	})
	done()
	require.NoError(t, err)
	require.ErrorIs(t, service.CancelMealAllocation(ctx, legacy), domain.ErrAllocationIdentityMissing)
	// Restore the fixture quantity reserved by the legacy operation before racing a new revision.
	changed, err := func() (bool, error) {
		operationCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		var changed bool
		err := unit.ExecuteWithRepositoryProvider(operationCtx, func(p *inventorymysql.RepositoryProvider) error {
			var err error
			changed, err = p.Allocation(operationCtx).Cancel(legacy.Revision)
			return err
		})
		return changed, err
	}()
	require.NoError(t, err)
	require.True(t, changed)
	race := identity
	race.Revision = uuid.New()
	raceRequest := app.MealAllocation{
		PlanID:         race.PlanID,
		ItemID:         race.ItemID,
		Revision:       race.Revision,
		ActorID:        race.ActorID,
		ShoppingListID: race.ShoppingListID,
		InventoryIDs:   race.InventoryIDs,
		Items:          []domain.ReservationItem{{ProductID: product.ID(), Quantity: 2}},
	}
	gate := make(chan struct{})
	results := make(chan error, 2)
	var workers sync.WaitGroup
	for _, cancelRequest := range []bool{false, true} {
		workers.Add(1)
		go func(cancelRequest bool) {
			defer workers.Done()
			<-gate
			operationCtx, done := context.WithCancel(ctx)
			defer done()
			if cancelRequest {
				results <- service.CancelMealAllocation(operationCtx, race)
				return
			}
			results <- service.AllocateMeal(operationCtx, raceRequest)
		}(cancelRequest)
	}
	close(gate)
	workers.Wait()
	close(results)
	for err := range results {
		require.NoError(t, err)
	}
	released, err = service.GetStock(ctx, inventory.ID(), product.ID())
	require.NoError(t, err)
	require.Zero(t, released.ReservedQuantity)
}

package persistence_tests

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"uuid"

	inventoryevents "inventory/api/events"
	mealplannerevents "mealplanner/api/events"
	"shopping/internal/shopping/app"
	"shopping/internal/shopping/domain"
	handler "shopping/internal/shopping/infra/handlers/events"
	shoppingdb "shopping/internal/shopping/infra/mysql"

	"github.com/distributed-programming-2026/go-sdk/pkg/event"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	"github.com/stretchr/testify/require"
)

func testShortageInbox(
	ctx context.Context,
	t *testing.T,
	client mysql.TransactionalClient,
	unit uow.UnitOfWorkWithRepositoryProvider[*shoppingdb.RepositoryProvider],
	service *app.Service,
	owner, list uuid.UUID,
) {
	t.Helper()
	source := domain.Source{MealPlanID: uuid.New(), MealItemID: uuid.New(), Revision: uuid.New()}
	product := uuid.New()
	payload := &inventoryevents.AllocationCompleted{
		PlanId:         source.MealPlanID.String(),
		ItemId:         source.MealItemID.String(),
		Revision:       source.Revision.String(),
		ActorId:        owner.String(),
		ShoppingListId: list.String(),
		ResultVersion:  1,
		Shortages:      []*inventoryevents.ReservationLine{{ProductId: product.String(), Quantity: 3}},
	}
	envelope, err := event.New(ctx, "inventory", "AllocationCompleted", payload)
	require.NoError(t, err)
	base := handler.NewHandler(slog.Default(), service)
	failure := errors.New("failure after demand")
	failing := event.HandlerFunc(func(ctx context.Context, envelope event.Envelope) error {
		if err := base.Handle(ctx, envelope); err != nil {
			return err
		}
		return failure
	})
	consumer := "shopping-test-" + uuid.New().String()
	wrapped, err := handler.WithInbox(failing, unit, consumer)
	require.NoError(t, err)
	var before, after, inboxCount int
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.outbox.count", `SELECT COUNT(*) FROM event_outbox`).Scan(&before),
	)
	require.ErrorIs(t, wrapped.Handle(ctx, envelope), failure)
	demands, err := shoppingdb.NewRepositoryProvider(client).Demand(ctx).ListBySource(list, source)
	require.NoError(t, err)
	require.Empty(t, demands)
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.outbox.count", `SELECT COUNT(*) FROM event_outbox`).Scan(&after),
	)
	require.Equal(t, before, after)
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.inbox.count", `SELECT COUNT(*) FROM event_inbox WHERE consumer = ? AND event_id = ?`, consumer, envelope.ID()).
			Scan(&inboxCount),
	)
	require.Zero(t, inboxCount)
	wrapped, err = handler.WithInbox(base, unit, consumer)
	require.NoError(t, err)
	require.NoError(t, wrapped.Handle(ctx, envelope))
	require.NoError(t, wrapped.Handle(ctx, envelope))
	demands, err = shoppingdb.NewRepositoryProvider(client).Demand(ctx).ListBySource(list, source)
	require.NoError(t, err)
	require.Len(t, demands, 1)
	require.Equal(t, product, demands[0].Data().ProductID)
	require.EqualValues(t, 3, demands[0].Remaining())
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.outbox.count", `SELECT COUNT(*) FROM event_outbox`).Scan(&after),
	)
	require.Equal(t, before+1, after)
	duplicate, err := event.New(ctx, "inventory", "AllocationCompleted", payload)
	require.NoError(t, err)
	require.NoError(t, wrapped.Handle(ctx, duplicate))
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.outbox.count", `SELECT COUNT(*) FROM event_outbox`).Scan(&after),
	)
	require.Equal(t, before+1, after)
	payload.Shortages[0].Quantity = 4
	conflict, err := event.New(ctx, "inventory", "AllocationCompleted", payload)
	require.NoError(t, err)
	require.ErrorIs(t, wrapped.Handle(ctx, conflict), domain.ErrDemandConflict)

	demandID := demands[0].Data().ID
	_, err = client.ExecContext(
		ctx,
		"test.purchase",
		`UPDATE shopping_demands SET purchased = 2 WHERE id = ?`,
		demandID[:],
	)
	require.NoError(t, err)
	manualID, manualProduct := uuid.New(), uuid.New()
	_, err = client.ExecContext(
		ctx,
		"test.manual",
		`INSERT INTO shopping_demands (id,list_id,product_id,required,purchased,cancelled) VALUES (?, ?, ?, 5, 0, FALSE)`,
		manualID[:],
		list[:],
		manualProduct[:],
	)
	require.NoError(t, err)
	require.ErrorIs(t, service.CancelShortages(ctx, uuid.New(), list, source), domain.ErrListNotFound)
	cancelCtx, done := context.WithCancel(ctx)
	rollback := errors.New("cancel rollback")
	err = unit.ExecuteWithRepositoryProvider(cancelCtx, func(_ *shoppingdb.RepositoryProvider) error {
		if err := service.CancelShortages(cancelCtx, owner, list, source); err != nil {
			return err
		}
		return rollback
	})
	done()
	require.ErrorIs(t, err, rollback)
	demands, err = shoppingdb.NewRepositoryProvider(client).Demand(ctx).ListBySource(list, source)
	require.NoError(t, err)
	require.False(t, demands[0].Data().Cancelled)
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.outbox.count", `SELECT COUNT(*) FROM event_outbox`).Scan(&after),
	)
	require.Equal(t, before+1, after)
	cancellation, err := event.New(
		ctx,
		"mealplanner",
		"MealCancellationRequested",
		&mealplannerevents.MealCancellationRequested{
			PlanId:         source.MealPlanID.String(),
			ItemId:         source.MealItemID.String(),
			Revision:       source.Revision.String(),
			ActorId:        owner.String(),
			ShoppingListId: list.String(),
		},
	)
	require.NoError(t, err)
	require.NoError(t, wrapped.Handle(ctx, cancellation))
	require.NoError(t, wrapped.Handle(ctx, cancellation))
	require.NoError(t, service.CancelShortages(ctx, owner, list, source))
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.outbox.count", `SELECT COUNT(*) FROM event_outbox`).Scan(&after),
	)
	require.Equal(t, before+3, after)
	wrongSource := source
	wrongSource.MealItemID = uuid.New()
	require.ErrorIs(t, service.CancelShortages(ctx, owner, list, wrongSource), domain.ErrDemandConflict)
	demands, err = shoppingdb.NewRepositoryProvider(client).Demand(ctx).ListBySource(list, source)
	require.NoError(t, err)
	require.Len(t, demands, 1)
	require.True(t, demands[0].Data().Cancelled)
	require.EqualValues(t, 2, demands[0].Data().Purchased)
	require.Zero(t, demands[0].Remaining())
	var manualCancelled bool
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.manual", `SELECT cancelled FROM shopping_demands WHERE id = ?`, manualID[:]).
			Scan(&manualCancelled),
	)
	require.False(t, manualCancelled)

	other := domain.Source{MealPlanID: uuid.New(), MealItemID: uuid.New(), Revision: uuid.New()}
	otherResult := app.AllocationResult{
		ActorID:   owner,
		ListID:    list,
		Source:    other,
		Version:   1,
		Shortages: []app.Shortage{{ProductID: product, Quantity: 6}},
	}
	require.NoError(t, service.RecordShortages(ctx, otherResult))
	require.NoError(t, service.CancelShortages(ctx, owner, list, source))
	otherDemands, err := shoppingdb.NewRepositoryProvider(client).Demand(ctx).ListBySource(list, other)
	require.NoError(t, err)
	require.Len(t, otherDemands, 1)
	require.EqualValues(t, 6, otherDemands[0].Remaining())

	early := domain.Source{MealPlanID: uuid.New(), MealItemID: uuid.New(), Revision: uuid.New()}
	require.NoError(t, service.CancelShortages(ctx, owner, list, early))
	late := app.AllocationResult{
		ActorID:   owner,
		ListID:    list,
		Source:    early,
		Version:   1,
		Shortages: []app.Shortage{{ProductID: product, Quantity: 8}},
	}
	require.NoError(t, service.RecordShortages(ctx, late))
	demands, err = shoppingdb.NewRepositoryProvider(client).Demand(ctx).ListBySource(list, early)
	require.NoError(t, err)
	require.Empty(t, demands)

	race := domain.Source{MealPlanID: uuid.New(), MealItemID: uuid.New(), Revision: uuid.New()}
	raceResult := app.AllocationResult{
		ActorID:   owner,
		ListID:    list,
		Source:    race,
		Version:   1,
		Shortages: []app.Shortage{{ProductID: product, Quantity: 4}},
	}
	gate := make(chan struct{})
	failures := make(chan error, 2)
	var workers sync.WaitGroup
	for _, cancelRequest := range []bool{false, true} {
		workers.Add(1)
		go func(cancelRequest bool) {
			defer workers.Done()
			<-gate
			operationCtx, done := context.WithCancel(ctx)
			defer done()
			if cancelRequest {
				failures <- service.CancelShortages(operationCtx, owner, list, race)
				return
			}
			failures <- service.RecordShortages(operationCtx, raceResult)
		}(cancelRequest)
	}
	close(gate)
	workers.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	demands, err = shoppingdb.NewRepositoryProvider(client).Demand(ctx).ListBySource(list, race)
	require.NoError(t, err)
	for _, demand := range demands {
		require.True(t, demand.Data().Cancelled)
		require.Zero(t, demand.Remaining())
	}
}

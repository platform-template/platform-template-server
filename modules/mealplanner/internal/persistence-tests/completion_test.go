package persistence_tests

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
	"uuid"

	inventoryevents "inventory/api/events"
	"mealplanner/internal/mealplanner/app"
	"mealplanner/internal/mealplanner/domain"
	handler "mealplanner/internal/mealplanner/infra/handlers/events"
	mealdb "mealplanner/internal/mealplanner/infra/mysql"

	"github.com/distributed-programming-2026/go-sdk/pkg/event"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	"github.com/stretchr/testify/require"
)

func testCompletionInbox(
	ctx context.Context,
	t *testing.T,
	client mysql.TransactionalClient,
	unit uow.UnitOfWorkWithRepositoryProvider[*mealdb.RepositoryProvider],
	service *app.Service,
	owner, planID uuid.UUID,
) {
	t.Helper()
	product, inventory, list, revision := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	item, err := domain.NewItem(domain.ItemData{
		ID: uuid.New(), PlanID: planID, Kind: domain.Product, ScheduledAt: time.Now(),
		Ingredients: []domain.Ingredient{{ProductID: product, Quantity: 2}}, InventoryIDs: []uuid.UUID{inventory},
		ShoppingListID: list, Revision: revision,
	})
	require.NoError(t, err)
	require.NoError(t, unit.ExecuteWithRepositoryProvider(ctx, func(p *mealdb.RepositoryProvider) error {
		return p.Item(ctx).Create(item)
	}))
	require.NoError(t, service.ApplyAllocation(ctx, app.AllocationResult{
		PlanID:         planID,
		ItemID:         item.Data().ID,
		Revision:       revision,
		ActorID:        owner,
		ShoppingListID: list,
		Version:        1,
		Allocations: []domain.AllocationLine{
			{InventoryID: inventory, ReservationID: uuid.New(), ProductID: product, Quantity: 2},
		},
	}))
	var before, after int
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.outbox.count", `SELECT COUNT(*) FROM event_outbox`).Scan(&before),
	)
	failure := errors.New("completion rollback")
	operationCtx, done := context.WithCancel(ctx)
	err = unit.ExecuteWithRepositoryProvider(operationCtx, func(_ *mealdb.RepositoryProvider) error {
		if err := service.CompleteItem(operationCtx, owner, planID, item.Data().ID); err != nil {
			return err
		}
		return failure
	})
	done()
	require.ErrorIs(t, err, failure)
	stored, _, err := mealdb.NewRepositoryProvider(client).Item(ctx).Get(planID, item.Data().ID)
	require.NoError(t, err)
	require.Equal(t, domain.Ready, stored.Data().State)
	require.NoError(t, service.CompleteItem(ctx, owner, planID, item.Data().ID))
	require.NoError(t, service.CompleteItem(ctx, owner, planID, item.Data().ID))
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.outbox.count", `SELECT COUNT(*) FROM event_outbox`).Scan(&after),
	)
	require.Equal(t, before+2, after)
	stored, _, err = mealdb.NewRepositoryProvider(client).Item(ctx).Get(planID, item.Data().ID)
	require.NoError(t, err)
	require.Equal(t, domain.Completing, stored.Data().State)
	wrapped, err := handler.WithInbox(
		handler.NewHandler(slog.Default(), service),
		unit,
		"completion-test-"+uuid.New().String(),
	)
	require.NoError(t, err)
	ack := &inventoryevents.AllocationConsumed{
		ActorId:        owner.String(),
		PlanId:         planID.String(),
		ItemId:         item.Data().ID.String(),
		Revision:       revision.String(),
		ShoppingListId: list.String(),
	}
	envelope, err := event.New(ctx, "inventory", "AllocationConsumed", ack)
	require.NoError(t, err)
	require.NoError(t, wrapped.Handle(ctx, envelope))
	require.NoError(t, wrapped.Handle(ctx, envelope))
	stored, _, err = mealdb.NewRepositoryProvider(client).Item(ctx).Get(planID, item.Data().ID)
	require.NoError(t, err)
	require.Equal(t, domain.Completed, stored.Data().State)
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.outbox.count", `SELECT COUNT(*) FROM event_outbox`).Scan(&after),
	)
	require.Equal(t, before+3, after)
}

package persistence_tests

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"uuid"

	inventoryevents "inventory/api/events"
	"mealplanner/internal/mealplanner/app"
	"mealplanner/internal/mealplanner/domain"
	handler "mealplanner/internal/mealplanner/infra/handlers/events"
	mealdb "mealplanner/internal/mealplanner/infra/mysql"
	shoppingevents "shopping/api/events"

	"github.com/distributed-programming-2026/go-sdk/pkg/event"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	"github.com/stretchr/testify/require"
)

func testCancellationInbox(
	ctx context.Context,
	t *testing.T,
	client mysql.TransactionalClient,
	unit uow.UnitOfWorkWithRepositoryProvider[*mealdb.RepositoryProvider],
	service *app.Service,
	owner uuid.UUID,
	item *domain.Item,
) {
	t.Helper()
	data := item.Data()
	require.ErrorIs(t, service.CancelItem(ctx, uuid.New(), data.PlanID, data.ID), domain.ErrPlanNotFound)
	failure := errors.New("cancel rollback")
	operationCtx, done := context.WithCancel(ctx)
	err := unit.ExecuteWithRepositoryProvider(operationCtx, func(_ *mealdb.RepositoryProvider) error {
		if err := service.CancelItem(operationCtx, owner, data.PlanID, data.ID); err != nil {
			return err
		}
		return failure
	})
	done()
	require.ErrorIs(t, err, failure)
	stored, _, err := mealdb.NewRepositoryProvider(client).Item(ctx).Get(data.PlanID, data.ID)
	require.NoError(t, err)
	require.Equal(t, domain.Missing, stored.Data().State)
	var before, after int
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.outbox.count", `SELECT COUNT(*) FROM event_outbox`).Scan(&before),
	)
	require.NoError(t, service.CancelItem(ctx, owner, data.PlanID, data.ID))
	require.NoError(t, service.CancelItem(ctx, owner, data.PlanID, data.ID))
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.outbox.count", `SELECT COUNT(*) FROM event_outbox`).Scan(&after),
	)
	require.Equal(t, before+2, after)
	stored, _, err = mealdb.NewRepositoryProvider(client).Item(ctx).Get(data.PlanID, data.ID)
	require.NoError(t, err)
	require.Equal(t, domain.Cancelling, stored.Data().State)
	wrapped, err := handler.WithInbox(
		handler.NewHandler(slog.Default(), service),
		unit,
		"cancel-test-"+uuid.New().String(),
	)
	require.NoError(t, err)
	shopping := &shoppingevents.ShortagesCancelled{
		ActorId:        owner.String(),
		PlanId:         data.PlanID.String(),
		ItemId:         data.ID.String(),
		Revision:       data.Revision.String(),
		ShoppingListId: data.ShoppingListID.String(),
	}
	first, err := event.New(ctx, "shopping", "ShortagesCancelled", shopping)
	require.NoError(t, err)
	require.NoError(t, wrapped.Handle(ctx, first))
	require.NoError(t, wrapped.Handle(ctx, first))
	stored, _, err = mealdb.NewRepositoryProvider(client).Item(ctx).Get(data.PlanID, data.ID)
	require.NoError(t, err)
	require.Equal(t, domain.Cancelling, stored.Data().State)
	require.True(t, stored.Data().ShoppingCancellationConfirmed)
	inventory := &inventoryevents.AllocationCancelled{
		ActorId:        owner.String(),
		PlanId:         data.PlanID.String(),
		ItemId:         data.ID.String(),
		Revision:       data.Revision.String(),
		ShoppingListId: data.ShoppingListID.String(),
	}
	second, err := event.New(ctx, "inventory", "AllocationCancelled", inventory)
	require.NoError(t, err)
	require.NoError(t, wrapped.Handle(ctx, second))
	require.NoError(t, wrapped.Handle(ctx, second))
	stored, _, err = mealdb.NewRepositoryProvider(client).Item(ctx).Get(data.PlanID, data.ID)
	require.NoError(t, err)
	require.Equal(t, domain.Cancelled, stored.Data().State)
	require.True(t, stored.Data().InventoryCancellationConfirmed)
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.outbox.count", `SELECT COUNT(*) FROM event_outbox`).Scan(&after),
	)
	require.Equal(t, before+3, after)
}

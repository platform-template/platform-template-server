package persistence_tests

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"uuid"

	inventoryevents "inventory/api/events"
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
}

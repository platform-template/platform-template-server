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

	"github.com/distributed-programming-2026/go-sdk/pkg/event"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	"github.com/stretchr/testify/require"
)

func testAllocationInbox(
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
	reservation := uuid.New()
	payload := &inventoryevents.AllocationCompleted{
		PlanId:         data.PlanID.String(),
		ItemId:         data.ID.String(),
		Revision:       data.Revision.String(),
		ActorId:        owner.String(),
		ShoppingListId: data.ShoppingListID.String(),
		ResultVersion:  1,
		Allocations: []*inventoryevents.AllocationLine{
			{
				InventoryId:   data.InventoryIDs[0].String(),
				ReservationId: reservation.String(),
				ProductId:     data.Ingredients[0].ProductID.String(),
				Quantity:      3,
			},
		},
		Shortages: []*inventoryevents.ReservationLine{{ProductId: data.Ingredients[0].ProductID.String(), Quantity: 2}},
	}
	envelope, err := event.New(ctx, "inventory", "AllocationCompleted", payload)
	require.NoError(t, err)
	base := handler.NewHandler(slog.Default(), service)
	failure := errors.New("fail after update")
	failing := event.HandlerFunc(func(ctx context.Context, envelope event.Envelope) error {
		if err := base.Handle(ctx, envelope); err != nil {
			return err
		}
		return failure
	})
	consumer := "meal-result-test-" + uuid.New().String()
	wrapped, err := handler.WithInbox(failing, unit, consumer)
	require.NoError(t, err)
	var before, after, inboxCount int
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.outbox.count", `SELECT COUNT(*) FROM event_outbox`).Scan(&before),
	)
	require.ErrorIs(t, wrapped.Handle(ctx, envelope), failure)
	stored, _, err := mealdb.NewRepositoryProvider(client).Item(ctx).Get(data.PlanID, data.ID)
	require.NoError(t, err)
	require.Equal(t, domain.Planning, stored.Data().State)
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
	stored, _, err = mealdb.NewRepositoryProvider(client).Item(ctx).Get(data.PlanID, data.ID)
	require.NoError(t, err)
	require.Equal(t, domain.Missing, stored.Data().State)
	require.EqualValues(t, 2, stored.Data().Shortages[0].Quantity)
	require.Equal(t, reservation, stored.Data().Allocations[0].ReservationID)
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.outbox.count", `SELECT COUNT(*) FROM event_outbox`).Scan(&after),
	)
	require.Equal(t, before+1, after)
	payload.Revision = uuid.New().String()
	old, err := event.New(ctx, "inventory", "AllocationCompleted", payload)
	require.NoError(t, err)
	require.NoError(t, wrapped.Handle(ctx, old))
	stored, _, err = mealdb.NewRepositoryProvider(client).Item(ctx).Get(data.PlanID, data.ID)
	require.NoError(t, err)
	require.Equal(t, domain.Missing, stored.Data().State)
}

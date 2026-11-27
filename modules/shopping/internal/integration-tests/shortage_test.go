package integration_tests

import (
	"io"
	"log/slog"
	"testing"
	"time"
	"uuid"

	inventoryevents "inventory/api/events"
	mealplannerevents "mealplanner/api/events"

	"github.com/distributed-programming-2026/go-sdk/pkg/amqp"
	"github.com/distributed-programming-2026/go-sdk/pkg/event"
	eventamqp "github.com/distributed-programming-2026/go-sdk/pkg/event/amqp"
	domainevent "github.com/distributed-programming-2026/lib/event"
	"github.com/stretchr/testify/require"
)

func TestShortageDelivery(t *testing.T) {
	ctx := t.Context()
	client := testEnv.connector.TransactionalClient()
	owner, list, plan, item, revision, product := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	_, err := client.ExecContext(
		ctx,
		"fixture.shopping_list",
		`INSERT INTO shopping_lists (id, owner_id, name) VALUES (?, ?, ?)`,
		list[:],
		owner[:],
		"List",
	)
	require.NoError(t, err)
	connection := amqp.NewConnection(
		moduleID,
		amqp.ConnectionConfig{URL: testEnv.conf.AMQP.URL, ConnectTimeout: time.Second},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	defer func() { require.NoError(t, connection.Stop()) }()
	dispatcher, err := eventamqp.NewDispatcher(connection.Producer(domainevent.DomainExchange(), nil, nil))
	require.NoError(t, err)
	require.NoError(t, connection.Start())
	payload := &inventoryevents.AllocationCompleted{
		PlanId:         plan.String(),
		ItemId:         item.String(),
		Revision:       revision.String(),
		ActorId:        owner.String(),
		ShoppingListId: list.String(),
		ResultVersion:  1,
		Shortages:      []*inventoryevents.ReservationLine{{ProductId: product.String(), Quantity: 4}},
	}
	envelope, err := event.New(ctx, "inventory", "AllocationCompleted", payload)
	require.NoError(t, err)
	require.NoError(t, dispatcher.Dispatch(ctx, envelope))
	require.NoError(t, dispatcher.Dispatch(ctx, envelope))
	waitHandled(t, envelope, 2)
	var count int
	var quantity int64
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.demand.count", `SELECT COUNT(*) FROM shopping_demands WHERE list_id = ? AND revision = ?`, list[:], revision[:]).
			Scan(&count),
	)
	require.Equal(t, 1, count)
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.demand.quantity", `SELECT required FROM shopping_demands WHERE list_id = ? AND revision = ?`, list[:], revision[:]).
			Scan(&quantity),
	)
	require.EqualValues(t, 4, quantity)
	cancelPayload := &mealplannerevents.MealCancellationRequested{
		PlanId:         plan.String(),
		ItemId:         item.String(),
		Revision:       revision.String(),
		ActorId:        owner.String(),
		ShoppingListId: list.String(),
	}
	cancellation, err := event.New(ctx, "mealplanner", "MealCancellationRequested", cancelPayload)
	require.NoError(t, err)
	require.NoError(t, dispatcher.Dispatch(ctx, cancellation))
	require.NoError(t, dispatcher.Dispatch(ctx, cancellation))
	waitHandled(t, cancellation, 2)
	var cancelled bool
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.demand.cancelled", `SELECT cancelled FROM shopping_demands WHERE list_id = ? AND revision = ?`, list[:], revision[:]).
			Scan(&cancelled),
	)
	require.True(t, cancelled)
}

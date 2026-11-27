package integration_tests

import (
	"io"
	"log/slog"
	"testing"
	"time"
	"uuid"

	mealplannerevents "mealplanner/api/events"

	"github.com/distributed-programming-2026/go-sdk/pkg/amqp"
	"github.com/distributed-programming-2026/go-sdk/pkg/event"
	eventamqp "github.com/distributed-programming-2026/go-sdk/pkg/event/amqp"
	domainevent "github.com/distributed-programming-2026/lib/event"
	"github.com/stretchr/testify/require"
)

func TestMealPlanningDelivery(t *testing.T) {
	ctx := t.Context()
	client := testEnv.connector.TransactionalClient()
	inventoryID, ownerID, productID := uuid.New(), uuid.New(), uuid.New()
	_, err := client.ExecContext(
		ctx,
		"fixture.inventory",
		`INSERT INTO inventories (id, name) VALUES (?, ?)`,
		inventoryID[:],
		"Meal test",
	)
	require.NoError(t, err)
	_, err = client.ExecContext(
		ctx,
		"fixture.owner",
		`INSERT INTO inventory_owners (inventory_id, user_id) VALUES (?, ?)`,
		inventoryID[:],
		ownerID[:],
	)
	require.NoError(t, err)
	_, err = client.ExecContext(
		ctx,
		"fixture.product",
		`INSERT INTO products (id, name, unit) VALUES (?, ?, ?)`,
		productID[:],
		"Meal product",
		"g",
	)
	require.NoError(t, err)
	_, err = client.ExecContext(
		ctx,
		"fixture.stock",
		`INSERT INTO stocks (inventory_id, product_id, quantity, reserved_quantity) VALUES (?, ?, ?, 0)`,
		inventoryID[:],
		productID[:],
		3,
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
	payload := &mealplannerevents.MealPlanningRequested{
		PlanId:         uuid.New().String(),
		ItemId:         uuid.New().String(),
		Revision:       uuid.New().String(),
		ActorId:        ownerID.String(),
		ShoppingListId: uuid.New().String(),
		InventoryIds:   []string{inventoryID.String()},
		Ingredients:    []*mealplannerevents.MealIngredient{{ProductId: productID.String(), Quantity: 5}},
	}
	envelope, err := event.New(ctx, "mealplanner", "MealPlanningRequested", payload)
	require.NoError(t, err)
	require.NoError(t, dispatcher.Dispatch(ctx, envelope))
	require.NoError(t, dispatcher.Dispatch(ctx, envelope))
	waitHandled(t, envelope, 2)
	var reserved int64
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.stock", `SELECT reserved_quantity FROM stocks WHERE inventory_id = ? AND product_id = ?`, inventoryID[:], productID[:]).
			Scan(&reserved),
	)
	require.EqualValues(t, 3, reserved)
	revision := uuid.MustParse(payload.Revision)
	var count int
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.allocations", `SELECT COUNT(*) FROM allocation_operations WHERE id = ?`, revision[:]).
			Scan(&count),
	)
	require.Equal(t, 1, count)
	cancelPayload := &mealplannerevents.MealCancellationRequested{
		PlanId:         payload.PlanId,
		ItemId:         payload.ItemId,
		Revision:       payload.Revision,
		ActorId:        payload.ActorId,
		ShoppingListId: payload.ShoppingListId,
		InventoryIds:   payload.InventoryIds,
	}
	cancellation, err := event.New(ctx, "mealplanner", "MealCancellationRequested", cancelPayload)
	require.NoError(t, err)
	require.NoError(t, dispatcher.Dispatch(ctx, cancellation))
	require.NoError(t, dispatcher.Dispatch(ctx, cancellation))
	waitHandled(t, cancellation, 2)
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.stock", `SELECT reserved_quantity FROM stocks WHERE inventory_id = ? AND product_id = ?`, inventoryID[:], productID[:]).
			Scan(&reserved),
	)
	require.Zero(t, reserved)
}

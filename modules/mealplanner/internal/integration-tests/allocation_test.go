package integration_tests

import (
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"
	"uuid"

	inventoryevents "inventory/api/events"
	"mealplanner/internal/mealplanner/domain"

	"github.com/distributed-programming-2026/go-sdk/pkg/amqp"
	"github.com/distributed-programming-2026/go-sdk/pkg/event"
	eventamqp "github.com/distributed-programming-2026/go-sdk/pkg/event/amqp"
	domainevent "github.com/distributed-programming-2026/lib/event"
	"github.com/stretchr/testify/require"
)

func TestAllocationDelivery(t *testing.T) {
	ctx := t.Context()
	client := testEnv.connector.TransactionalClient()
	owner, plan, product, inventory, reservation := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	data := domain.ItemData{
		ID:             uuid.New(),
		PlanID:         plan,
		Kind:           domain.Product,
		ScheduledAt:    time.Now().UTC(),
		Ingredients:    []domain.Ingredient{{ProductID: product, Quantity: 5}},
		InventoryIDs:   []uuid.UUID{inventory},
		ShoppingListID: uuid.New(),
		Revision:       uuid.New(),
	}
	item, err := domain.NewItem(data)
	require.NoError(t, err)
	raw, err := json.Marshal(item.Data())
	require.NoError(t, err)
	_, err = client.ExecContext(
		ctx,
		"fixture.plan",
		`INSERT INTO meal_plans (id, owner_id, name) VALUES (?, ?, ?)`,
		plan[:],
		owner[:],
		"Plan",
	)
	require.NoError(t, err)
	_, err = client.ExecContext(
		ctx,
		"fixture.item",
		`INSERT INTO meal_items (id, plan_id, scheduled_at, payload, row_version) VALUES (?, ?, ?, ?, 1)`,
		data.ID[:],
		plan[:],
		data.ScheduledAt,
		raw,
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
		ItemId:         data.ID.String(),
		Revision:       data.Revision.String(),
		ActorId:        owner.String(),
		ShoppingListId: data.ShoppingListID.String(),
		ResultVersion:  1,
		Allocations: []*inventoryevents.AllocationLine{
			{
				InventoryId:   inventory.String(),
				ReservationId: reservation.String(),
				ProductId:     product.String(),
				Quantity:      3,
			},
		},
		Shortages: []*inventoryevents.ReservationLine{{ProductId: product.String(), Quantity: 2}},
	}
	envelope, err := event.New(ctx, "inventory", "AllocationCompleted", payload)
	require.NoError(t, err)
	require.NoError(t, dispatcher.Dispatch(ctx, envelope))
	require.NoError(t, dispatcher.Dispatch(ctx, envelope))
	waitHandled(t, envelope, 2)
	var stored []byte
	var version uint64
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.item", `SELECT payload, row_version FROM meal_items WHERE id = ?`, data.ID[:]).
			Scan(&stored, &version),
	)
	require.EqualValues(t, 2, version)
	var result domain.ItemData
	require.NoError(t, json.Unmarshal(stored, &result))
	require.Equal(t, domain.Missing, result.State)
	require.EqualValues(t, 2, result.Shortages[0].Quantity)
	require.Equal(t, reservation, result.Allocations[0].ReservationID)
}

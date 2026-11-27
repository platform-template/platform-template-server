//go:build integration

package migrations_test

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"uuid"

	inventoryevents "inventory/api/events"
	"inventory/internal/inventory/app"
	"inventory/internal/inventory/domain"
	handler "inventory/internal/inventory/infra/handlers/events"
	inventorymysql "inventory/internal/inventory/infra/mysql"
	shoppingevents "shopping/api/events"

	"github.com/distributed-programming-2026/go-sdk/pkg/event"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	"github.com/stretchr/testify/require"
)

func testPurchaseInbox(
	t *testing.T,
	service *app.Service,
	unit uow.UnitOfWorkWithRepositoryProvider[*inventorymysql.RepositoryProvider],
	client mysql.TransactionalClient,
) {
	t.Helper()

	ctx := t.Context()

	papichID := uuid.New()
	purchaseID := uuid.New()
	shoppingListID := uuid.New()
	planID := uuid.New()
	itemID := uuid.New()
	revision := uuid.New()

	inventory, err := service.CreateOwnedInventory(ctx, papichID, "Холодос Величайшего")
	require.NoError(t, err)

	product, err := service.CreateProduct(ctx, "Молоко", domain.UnitMillilitres)
	require.NoError(t, err)

	require.NoError(t, service.AddStock(ctx, inventory.ID(), product.ID(), 500))

	envelope, err := event.New(ctx, "shopping", "PurchaseConfirmed", &shoppingevents.PurchaseConfirmed{
		PurchaseId:     purchaseID.String(),
		InventoryId:    inventory.ID().String(),
		ProductId:      product.ID().String(),
		ActorId:        papichID.String(),
		Quantity:       1000,
		ShoppingListId: shoppingListID.String(),
		MealPlanId:     planID.String(),
		MealItemId:     itemID.String(),
		Revision:       revision.String(),
	})
	require.NoError(t, err)

	base := handler.NewHandler(slog.Default(), service)
	consumer := "purchase-test-" + uuid.New().String()

	failure := errors.New("fail after stock replenishment")
	failing := event.HandlerFunc(func(ctx context.Context, envelope event.Envelope) error {
		if err := base.Handle(ctx, envelope); err != nil {
			return err
		}

		return failure
	})

	wrapped, err := handler.WithInbox(failing, unit, consumer)
	require.NoError(t, err)

	var outboxBefore int
	require.NoError(t, client.QueryRowContext(
		ctx,
		"test.purchase.outbox_before",
		`SELECT COUNT(*) FROM event_outbox`,
	).Scan(&outboxBefore))

	require.ErrorIs(t, wrapped.Handle(ctx, envelope), failure)

	stock, err := service.GetStock(ctx, inventory.ID(), product.ID())
	require.NoError(t, err)
	require.NotNil(t, stock)
	require.EqualValues(t, 500, stock.Quantity)

	var outboxAfter int
	require.NoError(t, client.QueryRowContext(
		ctx,
		"test.purchase.outbox_after",
		`SELECT COUNT(*) FROM event_outbox`,
	).Scan(&outboxAfter))
	require.Equal(t, outboxBefore, outboxAfter)

	var inboxCount int
	require.NoError(t, client.QueryRowContext(
		ctx,
		"test.purchase.inbox",
		`SELECT COUNT(*) FROM event_inbox WHERE consumer = ? AND event_id = ?`,
		consumer,
		envelope.ID(),
	).Scan(&inboxCount))
	require.Zero(t, inboxCount)

	var replenishmentCount int
	require.NoError(t, client.QueryRowContext(
		ctx,
		"test.purchase.replenishments",
		`SELECT COUNT(*) FROM stock_replenishment_operations WHERE id = ?`,
		purchaseID[:],
	).Scan(&replenishmentCount))
	require.Zero(t, replenishmentCount)

	wrapped, err = handler.WithInbox(base, unit, consumer)
	require.NoError(t, err)

	require.NoError(t, wrapped.Handle(ctx, envelope))
	require.NoError(t, wrapped.Handle(ctx, envelope))

	stock, err = service.GetStock(ctx, inventory.ID(), product.ID())
	require.NoError(t, err)
	require.NotNil(t, stock)
	require.EqualValues(t, 1500, stock.Quantity)

	require.NoError(t, client.QueryRowContext(
		ctx,
		"test.purchase.outbox_committed",
		`SELECT COUNT(*) FROM event_outbox`,
	).Scan(&outboxAfter))
	require.Equal(t, outboxBefore+3, outboxAfter)

	var payload []byte
	require.NoError(t, client.QueryRowContext(
		ctx,
		"test.purchase.replenished_event",
		`SELECT payload FROM event_outbox ORDER BY id DESC LIMIT 1`,
	).Scan(&payload))

	replenishedEnvelope, err := event.Unmarshal(payload)
	require.NoError(t, err)
	require.Equal(t, "inventory", replenishedEnvelope.Producer())
	require.Equal(t, "StockReplenished", replenishedEnvelope.Type())

	var replenished inventoryevents.StockReplenished
	require.NoError(t, replenishedEnvelope.UnmarshalPayload(&replenished))

	require.Equal(t, purchaseID.String(), replenished.OperationId)
	require.Equal(t, inventory.ID().String(), replenished.InventoryId)
	require.Equal(t, product.ID().String(), replenished.ProductId)
	require.Equal(t, papichID.String(), replenished.ActorId)
	require.EqualValues(t, 1000, replenished.Quantity)

	require.Equal(t, shoppingListID.String(), replenished.ShoppingListId)
	require.Equal(t, planID.String(), replenished.MealPlanId)
	require.Equal(t, itemID.String(), replenished.MealItemId)
	require.Equal(t, revision.String(), replenished.Revision)

	require.NoError(t, client.QueryRowContext(
		ctx,
		"test.purchase.inbox_committed",
		`SELECT COUNT(*) FROM event_inbox WHERE consumer = ? AND event_id = ?`,
		consumer,
		envelope.ID(),
	).Scan(&inboxCount))
	require.Equal(t, 1, inboxCount)

	require.NoError(t, client.QueryRowContext(
		ctx,
		"test.purchase.replenishments_committed",
		`SELECT COUNT(*) FROM stock_replenishment_operations WHERE id = ?`,
		purchaseID[:],
	).Scan(&replenishmentCount))
	require.Equal(t, 1, replenishmentCount)
}

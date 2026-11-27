package events

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"uuid"

	"inventory/internal/inventory/app"
	"inventory/internal/inventory/domain"
	shoppingevents "shopping/api/events"

	"github.com/distributed-programming-2026/go-sdk/pkg/event"
	"github.com/stretchr/testify/require"
)

type purchaseAllocator struct {
	ctx         context.Context
	operationID uuid.UUID
	inventoryID uuid.UUID
	productID   uuid.UUID
	actorID     uuid.UUID
	quantity    int64
}

func (a *purchaseAllocator) AllocateMeal(context.Context, app.MealAllocation) error {
	return nil
}

func (a *purchaseAllocator) CancelMealAllocation(context.Context, domain.AllocationIdentity) error {
	return nil
}

func (a *purchaseAllocator) CompleteMealAllocation(context.Context, domain.AllocationIdentity) error {
	return nil
}

func (a *purchaseAllocator) ReplenishStock(
	ctx context.Context,
	operationID, inventoryID, productID, actorID uuid.UUID,
	quantity int64,
) error {
	a.ctx = ctx
	a.operationID = operationID
	a.inventoryID = inventoryID
	a.productID = productID
	a.actorID = actorID
	a.quantity = quantity

	return nil
}

func TestPurchaseConfirmedReplenishesExactlyThePurchaseOperation(t *testing.T) {
	ctx := t.Context()

	purchaseID := uuid.New()
	inventoryID := uuid.New()
	productID := uuid.New()
	papichID := uuid.New()

	allocator := &purchaseAllocator{}
	handler := NewHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), allocator)

	envelope, err := event.New(ctx, "shopping", "PurchaseConfirmed", &shoppingevents.PurchaseConfirmed{
		PurchaseId:  purchaseID.String(),
		InventoryId: inventoryID.String(),
		ProductId:   productID.String(),
		ActorId:     papichID.String(),
		Quantity:    7,
	})
	require.NoError(t, err)

	require.NoError(t, handler.Handle(ctx, envelope))

	require.Equal(t, purchaseID, allocator.operationID)
	require.Equal(t, inventoryID, allocator.inventoryID)
	require.Equal(t, productID, allocator.productID)
	require.Equal(t, papichID, allocator.actorID)
	require.EqualValues(t, 7, allocator.quantity)
	require.True(t, ctx == allocator.ctx, "transaction context must remain unchanged")
}

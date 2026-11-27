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
	operation, inventory, product uuid.UUID
	quantity                      int64
}

func (a *purchaseAllocator) AllocateMeal(context.Context, app.MealAllocation) error { return nil }
func (a *purchaseAllocator) CancelMealAllocation(context.Context, domain.AllocationIdentity) error {
	return nil
}
func (a *purchaseAllocator) CompleteMealAllocation(context.Context, domain.AllocationIdentity) error {
	return nil
}
func (a *purchaseAllocator) ReplenishStock(
	_ context.Context,
	operation, inventory, product uuid.UUID,
	quantity int64,
) error {
	a.operation, a.inventory, a.product, a.quantity = operation, inventory, product, quantity
	return nil
}

func TestPurchaseConfirmedReplenishesExactlyThePurchaseOperation(t *testing.T) {
	operation, inventory, product, actor := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	allocator := &purchaseAllocator{}
	handler := NewHandler(slog.New(slog.NewTextHandler(io.Discard, nil)), allocator)
	envelope, err := event.New(
		t.Context(),
		"shopping",
		"PurchaseConfirmed",
		&shoppingevents.PurchaseConfirmed{
			PurchaseId:  operation.String(),
			InventoryId: inventory.String(),
			ProductId:   product.String(),
			ActorId:     actor.String(),
			Quantity:    7,
		},
	)
	require.NoError(t, err)
	require.NoError(t, handler.Handle(t.Context(), envelope))
	require.Equal(t, operation, allocator.operation)
	require.Equal(t, inventory, allocator.inventory)
	require.Equal(t, product, allocator.product)
	require.EqualValues(t, 7, allocator.quantity)
}

package app

import (
	"context"
	"fmt"
	"uuid"

	inventoryevents "inventory/api/events"
	"inventory/internal/inventory/domain"
	"inventory/internal/inventory/infra/mysql"
)

type MealPurchaseSource struct {
	ShoppingListID uuid.UUID
	PlanID         uuid.UUID
	ItemID         uuid.UUID
	Revision       uuid.UUID
}

func (s *Service) ReplenishStock(
	ctx context.Context,
	operationID, inventoryID, productID, actor uuid.UUID,
	quantity int64,
	source *MealPurchaseSource,
) error {
	if source != nil {
		if source.ShoppingListID == uuid.Nil() ||
			source.PlanID == uuid.Nil() ||
			source.ItemID == uuid.Nil() ||
			source.Revision == uuid.Nil() {
			return domain.ErrInvalidAllocation
		}
	}

	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		if err := requireResources(ctx, p, inventoryID, productID); err != nil {
			return err
		}

		changed, err := p.Stock(ctx).Replenish(operationID, inventoryID, productID, quantity)
		if err != nil || !changed {
			return err
		}

		actorID := ""
		if actor != uuid.Nil() {
			actorID = actor.String()
		}

		if err := s.dispatch(ctx, &inventoryevents.StockAdjusted{
			InventoryId:   inventoryID.String(),
			ProductId:     productID.String(),
			QuantityDelta: quantity,
			ActorId:       actorID,
		}); err != nil {
			return err
		}

		replenished := &inventoryevents.StockReplenished{
			OperationId: operationID.String(),
			InventoryId: inventoryID.String(),
			ProductId:   productID.String(),
			Quantity:    quantity,
			ActorId:     actorID,
		}

		if source != nil {
			replenished.ShoppingListId = source.ShoppingListID.String()
			replenished.MealPlanId = source.PlanID.String()
			replenished.MealItemId = source.ItemID.String()
			replenished.Revision = source.Revision.String()
		}

		return s.dispatcherFactory.NewDispatcher(ctx).Dispatch(replenished)
	})
	if err != nil {
		return fmt.Errorf("replenish stock: %w", err)
	}

	return nil
}

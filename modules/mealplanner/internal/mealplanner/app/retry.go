package app

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	mealplannerevents "mealplanner/api/events"
	"mealplanner/internal/mealplanner/domain"
	"mealplanner/internal/mealplanner/infra/mysql"
)

type PurchaseConfirmation struct {
	ActorID, PlanID, ItemID, Revision, ShoppingListID uuid.UUID
}

// RetryAfterPurchase starts compensation for an incomplete meal only. The next
// allocation receives a fresh revision after both participants confirm cleanup.
func (s *Service) RetryAfterPurchase(ctx context.Context, input PurchaseConfirmation) error {
	if input.ActorID == uuid.Nil() || input.PlanID == uuid.Nil() || input.ItemID == uuid.Nil() ||
		input.Revision == uuid.Nil() || input.ShoppingListID == uuid.Nil() {
		return domain.ErrInvalidItem
	}
	return s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		plan, err := p.Plan(ctx).Get(input.ActorID, input.PlanID)
		if err != nil {
			return err
		}
		if plan == nil {
			return domain.ErrPlanNotFound
		}
		item, version, err := p.Item(ctx).Get(input.PlanID, input.ItemID)
		if err != nil {
			return err
		}
		if item == nil {
			return domain.ErrInvalidItem
		}
		data := item.Data()
		if data.Revision != input.Revision || data.ShoppingListID != input.ShoppingListID ||
			data.State != domain.Missing {
			return nil
		}
		if err := item.RequestRetry(); err != nil {
			return err
		}
		if err := p.Item(ctx).Update(item, version); err != nil {
			return err
		}
		if s.dispatcherFactory == nil {
			return errors.New("event dispatcher is not configured")
		}
		return s.dispatchCancellation(ctx, input.ActorID, item)
	})
}

func (s *Service) dispatchCancellation(ctx context.Context, actor uuid.UUID, item *domain.Item) error {
	data := item.Data()
	request := &mealplannerevents.MealCancellationRequested{
		PlanId: data.PlanID.String(), ItemId: data.ID.String(), Revision: data.Revision.String(),
		ActorId: actor.String(), ShoppingListId: data.ShoppingListID.String(),
	}
	for _, id := range data.InventoryIDs {
		request.InventoryIds = append(request.InventoryIds, id.String())
	}
	return s.dispatcherFactory.NewDispatcher(ctx).Dispatch(request)
}

func (s *Service) dispatchPlanning(ctx context.Context, actor uuid.UUID, item *domain.Item) error {
	data := item.Data()
	request := &mealplannerevents.MealPlanningRequested{
		PlanId: data.PlanID.String(), ItemId: data.ID.String(), Revision: data.Revision.String(),
		ActorId: actor.String(), ShoppingListId: data.ShoppingListID.String(),
	}
	for _, id := range data.InventoryIDs {
		request.InventoryIds = append(request.InventoryIds, id.String())
	}
	for _, ingredient := range data.Ingredients {
		request.Ingredients = append(
			request.Ingredients,
			&mealplannerevents.MealIngredient{ProductId: ingredient.ProductID.String(), Quantity: ingredient.Quantity},
		)
	}
	if err := s.dispatcherFactory.NewDispatcher(ctx).Dispatch(request); err != nil {
		return fmt.Errorf("dispatch retry planning: %w", err)
	}
	return nil
}

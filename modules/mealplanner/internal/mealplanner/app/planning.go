package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"uuid"

	auditevents "audit/api/events"
	mealplannerevents "mealplanner/api/events"
	"mealplanner/internal/mealplanner/domain"
	"mealplanner/internal/mealplanner/infra/mysql"
)

type InventoryAccess interface {
	CheckOwners(context.Context, uuid.UUID, []uuid.UUID) error
	ValidateProduct(context.Context, uuid.UUID) error
}
type ShoppingAccess interface {
	CheckOwner(context.Context, uuid.UUID, uuid.UUID) error
}
type RecipeSnapshots interface {
	Snapshot(context.Context, uuid.UUID, uuid.UUID, int64) (domain.RecipeSnapshot, error)
}
type PlanningDependencies struct {
	Inventory InventoryAccess
	Shopping  ShoppingAccess
	Recipes   RecipeSnapshots
}

func NewPlanningService(unitService *Service, deps PlanningDependencies) *Service {
	return &Service{unit: unitService.unit, dispatcherFactory: unitService.dispatcherFactory, planning: &deps}
}

type ScheduleInput struct {
	PlanID         uuid.UUID
	ShoppingListID uuid.UUID
	InventoryIDs   []uuid.UUID
	ScheduledAt    time.Time
	Kind           domain.ItemKind
	RecipeID       uuid.UUID
	Servings       int64
	ProductID      uuid.UUID
	Quantity       int64
}

func (s *Service) Schedule(ctx context.Context, owner uuid.UUID, input ScheduleInput) (*domain.Item, error) {
	if s.planning == nil || s.planning.Inventory == nil || s.planning.Shopping == nil || s.planning.Recipes == nil {
		return nil, errors.New("planning dependencies are not configured")
	}
	if _, err := s.GetPlan(ctx, owner, input.PlanID); err != nil {
		return nil, err
	}
	checkCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.planning.Inventory.CheckOwners(checkCtx, owner, input.InventoryIDs); err != nil {
		return nil, fmt.Errorf("check inventories: %w", err)
	}
	if err := s.planning.Shopping.CheckOwner(checkCtx, owner, input.ShoppingListID); err != nil {
		return nil, fmt.Errorf("check shopping list: %w", err)
	}
	item, err := s.prepareItem(checkCtx, owner, input)
	if err != nil {
		return nil, err
	}
	err = s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		plan, err := provider.Plan(ctx).Get(owner, input.PlanID)
		if err != nil {
			return err
		}
		if plan == nil {
			return domain.ErrPlanNotFound
		}
		if err := provider.Item(ctx).Create(item); err != nil {
			return err
		}
		if s.dispatcherFactory == nil {
			return errors.New("event dispatcher is not configured")
		}
		data := item.Data()
		event := &mealplannerevents.MealPlanningRequested{
			PlanId:         data.PlanID.String(),
			ItemId:         data.ID.String(),
			Revision:       data.Revision.String(),
			ActorId:        owner.String(),
			ShoppingListId: data.ShoppingListID.String(),
		}
		for _, id := range data.InventoryIDs {
			event.InventoryIds = append(event.InventoryIds, id.String())
		}
		for _, ingredient := range data.Ingredients {
			event.Ingredients = append(
				event.Ingredients,
				&mealplannerevents.MealIngredient{
					ProductId: ingredient.ProductID.String(),
					Quantity:  ingredient.Quantity,
				},
			)
		}
		dispatcher := s.dispatcherFactory.NewDispatcher(ctx)
		if err := dispatcher.Dispatch(event); err != nil {
			return err
		}
		metadata, err := json.Marshal(
			map[string]string{auditPlanIDKey: data.PlanID.String(), auditRevisionKey: data.Revision.String()},
		)
		if err != nil {
			return err
		}
		return dispatcher.Dispatch(
			&auditevents.AuditRecordRequested{
				ActorId:      owner.String(),
				Action:       "meal_item.planning_requested",
				TargetType:   auditMealItemTarget,
				TargetId:     data.ID.String(),
				MetadataJson: metadata,
			},
		)
	})
	if err != nil {
		return nil, fmt.Errorf("schedule meal item: %w", err)
	}
	return item, nil
}

func (s *Service) prepareItem(ctx context.Context, owner uuid.UUID, input ScheduleInput) (*domain.Item, error) {
	data := domain.ItemData{
		ID:             uuid.New(),
		PlanID:         input.PlanID,
		Kind:           input.Kind,
		ScheduledAt:    input.ScheduledAt,
		InventoryIDs:   input.InventoryIDs,
		ShoppingListID: input.ShoppingListID,
		Revision:       uuid.New(),
	}
	switch input.Kind {
	case domain.Dish:
		if input.ProductID != uuid.Nil() || input.Quantity != 0 {
			return nil, domain.ErrInvalidItem
		}
		snapshot, err := s.planning.Recipes.Snapshot(ctx, owner, input.RecipeID, input.Servings)
		if err != nil {
			return nil, fmt.Errorf("fetch recipe snapshot: %w", err)
		}
		if snapshot.RecipeID != input.RecipeID || snapshot.Servings != input.Servings {
			return nil, domain.ErrInvalidItem
		}
		data.RecipeID, data.RecipeName, data.RecipeDescription, data.RecipeInstructions, data.Servings, data.Ingredients = snapshot.RecipeID, snapshot.Name, snapshot.Description, snapshot.Instructions, snapshot.Servings, snapshot.Ingredients
	case domain.Product:
		if input.RecipeID != uuid.Nil() || input.Servings != 0 || input.Quantity <= 0 {
			return nil, domain.ErrInvalidItem
		}
		if err := s.planning.Inventory.ValidateProduct(ctx, input.ProductID); err != nil {
			return nil, fmt.Errorf("validate product: %w", err)
		}
		data.Ingredients = []domain.Ingredient{{ProductID: input.ProductID, Quantity: input.Quantity}}
	default:
		return nil, domain.ErrInvalidItem
	}
	return domain.NewItem(data)
}

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"uuid"

	auditevents "audit/api/events"
	"mealplanner/internal/mealplanner/domain"
	"mealplanner/internal/mealplanner/infra/mysql"
)

type AllocationResult struct {
	PlanID         uuid.UUID
	ItemID         uuid.UUID
	Revision       uuid.UUID
	ActorID        uuid.UUID
	ShoppingListID uuid.UUID
	Version        uint64
	Allocations    []domain.AllocationLine
	Shortages      []domain.Ingredient
}

func (s *Service) ApplyAllocation(ctx context.Context, result AllocationResult) error {
	if result.PlanID == uuid.Nil() || result.ItemID == uuid.Nil() || result.Revision == uuid.Nil() ||
		result.ActorID == uuid.Nil() ||
		result.ShoppingListID == uuid.Nil() ||
		result.Version == 0 {
		return domain.ErrInvalidItem
	}
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		plan, err := p.Plan(ctx).Get(result.ActorID, result.PlanID)
		if err != nil {
			return err
		}
		if plan == nil {
			return domain.ErrPlanNotFound
		}
		item, version, err := p.Item(ctx).Get(result.PlanID, result.ItemID)
		if err != nil {
			return err
		}
		if item == nil {
			return domain.ErrInvalidItem
		}
		if item.Data().Revision != result.Revision {
			return nil
		}
		if item.Data().ShoppingListID != result.ShoppingListID {
			return domain.ErrInvalidItem
		}
		changed, err := item.ApplyAllocation(result.Revision, result.Version, result.Allocations, result.Shortages)
		if err != nil || !changed {
			return err
		}
		if err := p.Item(ctx).Update(item, version); err != nil {
			return err
		}
		if s.dispatcherFactory == nil {
			return errors.New("event dispatcher is not configured")
		}
		metadata, err := json.Marshal(
			map[string]any{
				"planId":   result.PlanID.String(),
				"revision": result.Revision.String(),
				"state":    item.Data().State,
			},
		)
		if err != nil {
			return err
		}
		return s.dispatcherFactory.NewDispatcher(ctx).
			Dispatch(&auditevents.AuditRecordRequested{ActorId: result.ActorID.String(), Action: "meal_item.allocation_updated", TargetType: "meal_item", TargetId: result.ItemID.String(), MetadataJson: metadata})
	})
	if err != nil {
		return fmt.Errorf("apply meal allocation: %w", err)
	}
	return nil
}

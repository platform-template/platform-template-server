package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"uuid"

	auditevents "audit/api/events"
	mealplannerevents "mealplanner/api/events"
	"mealplanner/internal/mealplanner/domain"
	"mealplanner/internal/mealplanner/infra/mysql"
)

func (s *Service) CancelItem(ctx context.Context, actor, planID, itemID uuid.UUID) error {
	if actor == uuid.Nil() || planID == uuid.Nil() || itemID == uuid.Nil() {
		return domain.ErrInvalidItem
	}
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		plan, err := p.Plan(ctx).Get(actor, planID)
		if err != nil {
			return err
		}
		if plan == nil {
			return domain.ErrPlanNotFound
		}
		item, version, err := p.Item(ctx).Get(planID, itemID)
		if err != nil {
			return err
		}
		if item == nil {
			return domain.ErrInvalidItem
		}
		data := item.Data()
		if data.State == domain.Cancelled {
			return nil
		}
		if data.State == domain.Cancelling {
			if !item.CancelRetry() {
				return nil
			}
			return p.Item(ctx).Update(item, version)
		}
		if err := item.RequestCancellation(); err != nil {
			return err
		}
		if err := p.Item(ctx).Update(item, version); err != nil {
			return err
		}
		if s.dispatcherFactory == nil {
			return errors.New("event dispatcher is not configured")
		}
		request := &mealplannerevents.MealCancellationRequested{
			PlanId:         planID.String(),
			ItemId:         itemID.String(),
			Revision:       data.Revision.String(),
			ActorId:        actor.String(),
			ShoppingListId: data.ShoppingListID.String(),
		}
		for _, id := range data.InventoryIDs {
			request.InventoryIds = append(request.InventoryIds, id.String())
		}
		dispatcher := s.dispatcherFactory.NewDispatcher(ctx)
		if err := dispatcher.Dispatch(request); err != nil {
			return err
		}
		metadata, err := json.Marshal(
			map[string]string{auditPlanIDKey: planID.String(), auditRevisionKey: data.Revision.String()},
		)
		if err != nil {
			return err
		}
		return dispatcher.Dispatch(
			&auditevents.AuditRecordRequested{
				ActorId:      actor.String(),
				Action:       "meal_item.cancellation_requested",
				TargetType:   auditMealItemTarget,
				TargetId:     itemID.String(),
				MetadataJson: metadata,
			},
		)
	})
	if err != nil {
		return fmt.Errorf("cancel meal item: %w", err)
	}
	return nil
}

type CancellationConfirmation struct {
	ActorID, PlanID, ItemID, Revision, ShoppingListID uuid.UUID
	Participant                                       domain.CancellationParticipant
}

func (s *Service) ConfirmCancellation(ctx context.Context, input CancellationConfirmation) error {
	if input.ActorID == uuid.Nil() || input.PlanID == uuid.Nil() || input.ItemID == uuid.Nil() ||
		input.Revision == uuid.Nil() ||
		input.ShoppingListID == uuid.Nil() {
		return domain.ErrInvalidItem
	}
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
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
		if data.Revision != input.Revision {
			return nil
		}
		if data.ShoppingListID != input.ShoppingListID {
			return domain.ErrInvalidItem
		}
		changed, err := item.ConfirmCancellation(input.Revision, input.Participant)
		if err != nil || !changed {
			return err
		}
		if err := p.Item(ctx).Update(item, version); err != nil {
			return err
		}
		restarted, err := s.restartAfterCancellation(ctx, p, item, version, input.ActorID)
		if err != nil {
			return err
		}
		if restarted {
			return nil
		}
		if item.Data().State != domain.Cancelled {
			return nil
		}
		if s.dispatcherFactory == nil {
			return errors.New("event dispatcher is not configured")
		}
		metadata, err := json.Marshal(
			map[string]string{auditPlanIDKey: input.PlanID.String(), auditRevisionKey: input.Revision.String()},
		)
		if err != nil {
			return err
		}
		return s.dispatcherFactory.NewDispatcher(ctx).
			Dispatch(&auditevents.AuditRecordRequested{ActorId: input.ActorID.String(), Action: "meal_item.cancelled", TargetType: auditMealItemTarget, TargetId: input.ItemID.String(), MetadataJson: metadata})
	})
	if err != nil {
		return fmt.Errorf("confirm meal cancellation: %w", err)
	}
	return nil
}

func (s *Service) restartAfterCancellation(
	ctx context.Context,
	p *mysql.RepositoryProvider,
	item *domain.Item,
	version uint64,
	actor uuid.UUID,
) (bool, error) {
	data := item.Data()
	if !data.RetryRequested || !data.InventoryCancellationConfirmed || !data.ShoppingCancellationConfirmed {
		return false, nil
	}
	if !item.RestartAfterCancellation(uuid.New()) {
		return false, domain.ErrInvalidItem
	}
	if err := p.Item(ctx).Update(item, version+1); err != nil {
		return false, err
	}
	if s.dispatcherFactory == nil {
		return false, errors.New("event dispatcher is not configured")
	}
	if err := s.dispatchPlanning(ctx, actor, item); err != nil {
		return false, err
	}
	return true, nil
}

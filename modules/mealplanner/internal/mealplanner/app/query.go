package app

import (
	"context"
	"fmt"
	"time"
	"uuid"

	"mealplanner/internal/mealplanner/domain"
	"mealplanner/internal/mealplanner/infra/mysql"
)

// GetItem returns the current asynchronous state of an item after confirming
// that the requested plan belongs to the caller.
func (s *Service) GetItem(ctx context.Context, owner, planID, itemID uuid.UUID) (*domain.Item, error) {
	if owner == uuid.Nil() || planID == uuid.Nil() || itemID == uuid.Nil() {
		return nil, domain.ErrInvalidItem
	}
	var item *domain.Item
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		plan, err := p.Plan(ctx).Get(owner, planID)
		if err != nil {
			return err
		}
		if plan == nil {
			return domain.ErrPlanNotFound
		}
		item, _, err = p.Item(ctx).Get(planID, itemID)
		if err != nil {
			return err
		}
		if item == nil {
			return domain.ErrItemNotFound
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("get meal item: %w", err)
	}
	return item, nil
}

// ListCalendar returns the caller's items in the half-open [from, to) interval.
func (s *Service) ListCalendar(
	ctx context.Context,
	owner, planID uuid.UUID,
	from, to time.Time,
) ([]*domain.Item, error) {
	if owner == uuid.Nil() || planID == uuid.Nil() || from.IsZero() || to.IsZero() || !from.Before(to) {
		return nil, domain.ErrInvalidItem
	}
	var items []*domain.Item
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		plan, err := p.Plan(ctx).Get(owner, planID)
		if err != nil {
			return err
		}
		if plan == nil {
			return domain.ErrPlanNotFound
		}
		items, err = p.Item(ctx).List(planID, from, to)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list meal calendar: %w", err)
	}
	return items, nil
}

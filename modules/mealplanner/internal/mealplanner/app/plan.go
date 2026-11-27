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

func (s *Service) recordPlanAudit(ctx context.Context, actor uuid.UUID, plan *domain.Plan, action string) error {
	if s.dispatcherFactory == nil {
		return errors.New("event dispatcher is not configured")
	}
	metadata, err := json.Marshal(map[string]string{"name": plan.Name()})
	if err != nil {
		return fmt.Errorf("encode plan audit metadata: %w", err)
	}
	return s.dispatcherFactory.NewDispatcher(ctx).Dispatch(&auditevents.AuditRecordRequested{
		ActorId:      actor.String(),
		Action:       action,
		TargetType:   "meal_plan",
		TargetId:     plan.ID().String(),
		MetadataJson: metadata,
	})
}

func (s *Service) CreatePlan(ctx context.Context, owner uuid.UUID, name string) (*domain.Plan, error) {
	plan, err := domain.NewPlan(uuid.New(), owner, name)
	if err != nil {
		return nil, err
	}
	err = s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		if err := provider.Plan(ctx).Create(plan); err != nil {
			return err
		}
		return s.recordPlanAudit(ctx, owner, plan, "meal_plan.created")
	})
	if err != nil {
		return nil, fmt.Errorf("create meal plan: %w", err)
	}
	return plan, nil
}

func (s *Service) ListPlans(ctx context.Context, owner uuid.UUID) ([]*domain.Plan, error) {
	if owner == uuid.Nil() {
		return nil, domain.ErrInvalidPlan
	}
	var plans []*domain.Plan
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		var err error
		plans, err = provider.Plan(ctx).List(owner)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list meal plans: %w", err)
	}
	return plans, nil
}

func (s *Service) GetPlan(ctx context.Context, owner, planID uuid.UUID) (*domain.Plan, error) {
	if owner == uuid.Nil() || planID == uuid.Nil() {
		return nil, domain.ErrInvalidPlan
	}
	var plan *domain.Plan
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		var err error
		plan, err = provider.Plan(ctx).Get(owner, planID)
		if err != nil {
			return err
		}
		if plan == nil {
			return domain.ErrPlanNotFound
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("get meal plan: %w", err)
	}
	return plan, nil
}

func (s *Service) RenamePlan(ctx context.Context, owner, planID uuid.UUID, name string) error {
	if owner == uuid.Nil() || planID == uuid.Nil() {
		return domain.ErrInvalidPlan
	}
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		plan, err := provider.Plan(ctx).Get(owner, planID)
		if err != nil {
			return err
		}
		if plan == nil {
			return domain.ErrPlanNotFound
		}
		if plan.Name() == name {
			return nil
		}
		if err := plan.Rename(name); err != nil {
			return err
		}
		if err := provider.Plan(ctx).Update(plan); err != nil {
			return err
		}
		return s.recordPlanAudit(ctx, owner, plan, "meal_plan.renamed")
	})
	if err != nil {
		return fmt.Errorf("rename meal plan: %w", err)
	}
	return nil
}

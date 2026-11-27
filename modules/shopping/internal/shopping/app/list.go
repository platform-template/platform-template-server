package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"uuid"

	auditevents "audit/api/events"
	"shopping/internal/shopping/domain"
	"shopping/internal/shopping/infra/mysql"
)

func (s *Service) recordListAudit(ctx context.Context, actor uuid.UUID, plan *domain.List, action string) error {
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
		TargetType:   "shopping_list",
		TargetId:     plan.ID().String(),
		MetadataJson: metadata,
	})
}

func (s *Service) CreateList(ctx context.Context, owner uuid.UUID, name string) (*domain.List, error) {
	plan, err := domain.NewList(uuid.New(), owner, name)
	if err != nil {
		return nil, err
	}
	err = s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		if err := provider.List(ctx).Create(plan); err != nil {
			return err
		}
		return s.recordListAudit(ctx, owner, plan, "shopping_list.created")
	})
	if err != nil {
		return nil, fmt.Errorf("create shopping list: %w", err)
	}
	return plan, nil
}

func (s *Service) ListLists(ctx context.Context, owner uuid.UUID) ([]*domain.List, error) {
	if owner == uuid.Nil() {
		return nil, domain.ErrInvalidList
	}
	var plans []*domain.List
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		var err error
		plans, err = provider.List(ctx).List(owner)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list shopping lists: %w", err)
	}
	return plans, nil
}

func (s *Service) GetList(ctx context.Context, owner, planID uuid.UUID) (*domain.List, error) {
	if owner == uuid.Nil() || planID == uuid.Nil() {
		return nil, domain.ErrInvalidList
	}
	var plan *domain.List
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		var err error
		plan, err = provider.List(ctx).Get(owner, planID)
		if err != nil {
			return err
		}
		if plan == nil {
			return domain.ErrListNotFound
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("get shopping list: %w", err)
	}
	return plan, nil
}

func (s *Service) RenameList(ctx context.Context, owner, planID uuid.UUID, name string) error {
	if owner == uuid.Nil() || planID == uuid.Nil() {
		return domain.ErrInvalidList
	}
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		plan, err := provider.List(ctx).Get(owner, planID)
		if err != nil {
			return err
		}
		if plan == nil {
			return domain.ErrListNotFound
		}
		if plan.Name() == name {
			return nil
		}
		if err := plan.Rename(name); err != nil {
			return err
		}
		if err := provider.List(ctx).Update(plan); err != nil {
			return err
		}
		return s.recordListAudit(ctx, owner, plan, "shopping_list.renamed")
	})
	if err != nil {
		return fmt.Errorf("rename shopping list: %w", err)
	}
	return nil
}

package app

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	"uuid"

	"audit/internal/audit/domain"
	"audit/internal/audit/infra/mysql"
)

type RecordAuditEntryInput struct {
	EventID       string
	ActorID       *uuid.UUID
	ModuleKey     string
	Action        string
	TargetType    string
	TargetID      string
	CorrelationID string
	Metadata      json.RawMessage
	OccurredAt    time.Time
}

func (s *Service) RecordAuditEntry(ctx context.Context, input RecordAuditEntryInput) (*domain.AuditEntry, error) {
	entry, err := domain.NewAuditEntry(domain.AuditEntryData{
		EventID:       input.EventID,
		ActorID:       input.ActorID,
		ModuleKey:     input.ModuleKey,
		Action:        input.Action,
		TargetType:    input.TargetType,
		TargetID:      input.TargetID,
		CorrelationID: input.CorrelationID,
		Metadata:      input.Metadata,
		OccurredAt:    input.OccurredAt,
	})
	if err != nil {
		return nil, err
	}
	err = s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		return provider.Audit(ctx).Append(entry)
	})
	if err != nil {
		return nil, fmt.Errorf("record audit entry: %w", err)
	}
	return entry, nil
}

func (s *Service) GetAuditEntry(ctx context.Context, eventID string) (*domain.AuditEntry, error) {
	var entry *domain.AuditEntry
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		var err error
		entry, err = provider.Audit(ctx).GetByEventID(eventID)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("get audit entry: %w", err)
	}
	if entry == nil {
		return nil, domain.ErrAuditEntryNotFound
	}
	return entry, nil
}

func (s *Service) ListAuditEntries(ctx context.Context, filter domain.AuditFilter) ([]*domain.AuditEntry, error) {
	if filter.Limit < 0 || filter.Limit > 100 || filter.Offset < 0 ||
		(filter.ActorID != nil && *filter.ActorID == uuid.Nil()) ||
		(filter.From != nil && filter.From.IsZero()) || (filter.To != nil && filter.To.IsZero()) ||
		(filter.From != nil && filter.To != nil && filter.From.After(*filter.To)) {
		return nil, domain.ErrInvalidAuditFilter
	}
	if filter.Limit == 0 {
		filter.Limit = 20
	}
	var entries []*domain.AuditEntry
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		var err error
		entries, err = provider.Audit(ctx).List(filter)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list audit entries: %w", err)
	}
	if entries == nil {
		entries = make([]*domain.AuditEntry, 0)
	}
	return entries, nil
}

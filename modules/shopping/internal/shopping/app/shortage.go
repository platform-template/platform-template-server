package app

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"uuid"

	auditevents "audit/api/events"
	"shopping/internal/shopping/domain"
	"shopping/internal/shopping/infra/mysql"
)

type Shortage struct {
	ProductID uuid.UUID
	Quantity  int64
}
type AllocationResult struct {
	ActorID   uuid.UUID
	ListID    uuid.UUID
	Source    domain.Source
	Version   uint64
	Shortages []Shortage
}

func (s *Service) RecordShortages(ctx context.Context, result AllocationResult) error {
	if err := validateAllocationResult(result); err != nil {
		return err
	}
	sorted := result
	sorted.Shortages = append([]Shortage(nil), result.Shortages...)
	sort.Slice(sorted.Shortages, func(i, j int) bool {
		return sorted.Shortages[i].ProductID.String() < sorted.Shortages[j].ProductID.String()
	})
	encoded, err := json.Marshal(sorted)
	if err != nil {
		return fmt.Errorf("encode allocation result: %w", err)
	}
	hash := sha256.Sum256(encoded)
	err = s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		list, err := p.List(ctx).Get(result.ActorID, result.ListID)
		if err != nil {
			return err
		}
		if list == nil {
			return domain.ErrListNotFound
		}
		registered, err := p.Demand(ctx).RegisterAllocation(result.Source.Revision, hash)
		if err != nil || !registered {
			return err
		}
		created := 0
		for _, shortage := range result.Shortages {
			demand, err := domain.NewDemand(
				domain.DemandData{
					ID:        uuid.New(),
					ListID:    result.ListID,
					ProductID: shortage.ProductID,
					Required:  shortage.Quantity,
					Source:    &result.Source,
				},
			)
			if err != nil {
				return err
			}
			isNew, err := p.Demand(ctx).CreateAutomatic(demand)
			if err != nil {
				return err
			}
			if isNew {
				created++
			}
		}
		if created == 0 {
			return nil
		}
		if s.dispatcherFactory == nil {
			return errors.New("event dispatcher is not configured")
		}
		metadata, err := json.Marshal(
			map[string]any{
				"planId":      result.Source.MealPlanID.String(),
				"itemId":      result.Source.MealItemID.String(),
				"revision":    result.Source.Revision.String(),
				"demandCount": created,
			},
		)
		if err != nil {
			return err
		}
		return s.dispatcherFactory.NewDispatcher(ctx).
			Dispatch(&auditevents.AuditRecordRequested{ActorId: result.ActorID.String(), Action: "shopping_list.shortages_added", TargetType: "shopping_list", TargetId: result.ListID.String(), MetadataJson: metadata})
	})
	if err != nil {
		return fmt.Errorf("record shortages: %w", err)
	}
	return nil
}

func validateAllocationResult(result AllocationResult) error {
	if result.ActorID == uuid.Nil() || result.ListID == uuid.Nil() || result.Source.MealPlanID == uuid.Nil() ||
		result.Source.MealItemID == uuid.Nil() ||
		result.Source.Revision == uuid.Nil() ||
		result.Version != 1 {
		return domain.ErrInvalidDemand
	}
	seen := make(map[uuid.UUID]bool, len(result.Shortages))
	for _, shortage := range result.Shortages {
		if shortage.ProductID == uuid.Nil() || shortage.Quantity <= 0 || seen[shortage.ProductID] {
			return domain.ErrInvalidDemand
		}
		seen[shortage.ProductID] = true
	}
	return nil
}

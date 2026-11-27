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

func (s *Service) CancelShortages(ctx context.Context, actorID, listID uuid.UUID, source domain.Source) error {
	if actorID == uuid.Nil() || listID == uuid.Nil() || source.MealPlanID == uuid.Nil() ||
		source.MealItemID == uuid.Nil() ||
		source.Revision == uuid.Nil() {
		return domain.ErrInvalidDemand
	}
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		list, err := p.List(ctx).Get(actorID, listID)
		if err != nil {
			return err
		}
		if list == nil {
			return domain.ErrListNotFound
		}
		changed, err := p.Demand(ctx).CancelSource(listID, source)
		if err != nil || !changed {
			return err
		}
		if s.dispatcherFactory == nil {
			return errors.New("event dispatcher is not configured")
		}
		metadata, err := json.Marshal(
			map[string]string{
				"planId":   source.MealPlanID.String(),
				"itemId":   source.MealItemID.String(),
				"revision": source.Revision.String(),
			},
		)
		if err != nil {
			return err
		}
		return s.dispatcherFactory.NewDispatcher(ctx).
			Dispatch(&auditevents.AuditRecordRequested{ActorId: actorID.String(), Action: "shopping_list.shortages_cancelled", TargetType: auditListTarget, TargetId: listID.String(), MetadataJson: metadata})
	})
	if err != nil {
		return fmt.Errorf("cancel shortages: %w", err)
	}
	return nil
}

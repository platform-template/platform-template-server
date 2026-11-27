package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"uuid"

	auditevents "audit/api/events"
	"inventory/internal/inventory/domain"
	"inventory/internal/inventory/infra/mysql"
)

func (s *Service) CancelMealAllocation(ctx context.Context, identity domain.AllocationIdentity) error {
	if identity.Revision == uuid.Nil() || identity.ActorID == uuid.Nil() || identity.PlanID == uuid.Nil() ||
		identity.ItemID == uuid.Nil() ||
		identity.ShoppingListID == uuid.Nil() ||
		len(identity.InventoryIDs) == 0 {
		return domain.ErrInvalidAllocation
	}
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		if err := requireInventoryOwners(ctx, p, identity.ActorID, identity.InventoryIDs); err != nil {
			return err
		}
		if err := p.Allocation(ctx).BindIdentity(identity); err != nil {
			return err
		}
		changed, err := p.Allocation(ctx).Cancel(identity.Revision)
		if err != nil || !changed {
			return err
		}
		if s.dispatcherFactory == nil {
			return errors.New("inventory event dispatcher is not configured")
		}
		metadata, err := json.Marshal(
			map[string]string{"planId": identity.PlanID.String(), "revision": identity.Revision.String()},
		)
		if err != nil {
			return err
		}
		return s.dispatcherFactory.NewDispatcher(ctx).
			Dispatch(&auditevents.AuditRecordRequested{ActorId: identity.ActorID.String(), Action: "allocation.cancelled", TargetType: "meal_item", TargetId: identity.ItemID.String(), MetadataJson: metadata})
	})
	if err != nil {
		return fmt.Errorf("cancel meal allocation: %w", err)
	}
	return nil
}

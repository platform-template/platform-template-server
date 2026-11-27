package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"uuid"

	auditevents "audit/api/events"
	inventoryevents "inventory/api/events"
	"inventory/internal/inventory/domain"
	"inventory/internal/inventory/infra/mysql"
)

// CompleteMealAllocation consumes every reservation created for a fully allocated meal.
// The identity is checked again because delivery is asynchronous and untrusted at the edge.
func (s *Service) CompleteMealAllocation(ctx context.Context, identity domain.AllocationIdentity) error {
	if identity.Revision == uuid.Nil() || identity.ActorID == uuid.Nil() || identity.PlanID == uuid.Nil() ||
		identity.ItemID == uuid.Nil() || identity.ShoppingListID == uuid.Nil() || len(identity.InventoryIDs) == 0 {
		return domain.ErrInvalidAllocation
	}
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		if err := requireInventoryOwners(ctx, p, identity.ActorID, identity.InventoryIDs); err != nil {
			return err
		}
		if err := p.Allocation(ctx).BindIdentity(identity); err != nil {
			return err
		}
		changed, err := p.Allocation(ctx).Consume(identity.Revision)
		if errors.Is(err, domain.ErrAllocationCancelled) {
			return nil
		}
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
		dispatcher := s.dispatcherFactory.NewDispatcher(ctx)
		if err := dispatcher.Dispatch(&inventoryevents.AllocationConsumed{
			PlanId: identity.PlanID.String(), ItemId: identity.ItemID.String(), Revision: identity.Revision.String(),
			ActorId: identity.ActorID.String(), ShoppingListId: identity.ShoppingListID.String(),
		}); err != nil {
			return err
		}
		return dispatcher.Dispatch(&auditevents.AuditRecordRequested{
			ActorId: identity.ActorID.String(), Action: "allocation.consumed", TargetType: auditMealItemTarget,
			TargetId: identity.ItemID.String(), MetadataJson: metadata,
		})
	})
	if err != nil {
		return fmt.Errorf("complete meal allocation: %w", err)
	}
	return nil
}

package app

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	inventoryevents "inventory/api/events"
	"inventory/internal/inventory/domain"
	"inventory/internal/inventory/infra/mysql"
)

type MealAllocation struct {
	PlanID         uuid.UUID
	ItemID         uuid.UUID
	Revision       uuid.UUID
	ActorID        uuid.UUID
	ShoppingListID uuid.UUID
	InventoryIDs   []uuid.UUID
	Items          []domain.ReservationItem
}

func (s *Service) AllocateMeal(ctx context.Context, request MealAllocation) error {
	if request.PlanID == uuid.Nil() || request.ItemID == uuid.Nil() || request.Revision == uuid.Nil() ||
		request.ActorID == uuid.Nil() ||
		request.ShoppingListID == uuid.Nil() {
		return domain.ErrInvalidAllocation
	}
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		if err := requireInventoryOwners(ctx, p, request.ActorID, request.InventoryIDs); err != nil {
			return err
		}
		for _, item := range request.Items {
			product, err := p.Product(ctx).GetByID(item.ProductID)
			if err != nil {
				return err
			}
			if product == nil {
				return domain.ErrProductNotFound
			}
		}
		identity := domain.AllocationIdentity{
			Revision:       request.Revision,
			ActorID:        request.ActorID,
			PlanID:         request.PlanID,
			ItemID:         request.ItemID,
			ShoppingListID: request.ShoppingListID,
			InventoryIDs:   request.InventoryIDs,
		}
		if err := p.Allocation(ctx).BindIdentity(identity); err != nil {
			return err
		}
		result, created, err := p.Allocation(ctx).Allocate(request.Revision, request.InventoryIDs, request.Items)
		if errors.Is(err, domain.ErrAllocationCancelled) {
			return nil
		}
		if err != nil || !created {
			return err
		}
		if s.dispatcherFactory == nil {
			return errors.New("inventory event dispatcher is not configured")
		}
		payload := &inventoryevents.AllocationCompleted{
			PlanId:         request.PlanID.String(),
			ItemId:         request.ItemID.String(),
			Revision:       request.Revision.String(),
			ActorId:        request.ActorID.String(),
			ShoppingListId: request.ShoppingListID.String(),
			ResultVersion:  1,
		}
		for _, line := range result.Lines {
			payload.Allocations = append(
				payload.Allocations,
				&inventoryevents.AllocationLine{
					InventoryId:   line.InventoryID.String(),
					ReservationId: line.ReservationID.String(),
					ProductId:     line.ProductID.String(),
					Quantity:      line.Quantity,
				},
			)
		}
		for _, shortage := range result.Shortages {
			payload.Shortages = append(
				payload.Shortages,
				&inventoryevents.ReservationLine{ProductId: shortage.ProductID.String(), Quantity: shortage.Quantity},
			)
		}
		return s.dispatch(ctx, payload)
	})
	if err != nil {
		return fmt.Errorf("allocate meal: %w", err)
	}
	return nil
}

func requireInventoryOwners(
	ctx context.Context,
	p *mysql.RepositoryProvider,
	actor uuid.UUID,
	inventoryIDs []uuid.UUID,
) error {
	for _, inventoryID := range inventoryIDs {
		owned, err := p.InventoryOwnership(ctx).IsOwner(inventoryID, actor)
		if err != nil {
			return err
		}
		if !owned {
			return domain.ErrInventoryNotFound
		}
	}
	return nil
}

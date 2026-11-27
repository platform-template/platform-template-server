package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"uuid"

	auditevents "audit/api/events"
	shoppingevents "shopping/api/events"
	"shopping/internal/shopping/domain"
	"shopping/internal/shopping/infra/mysql"
)

func (s *Service) CreateManualDemand(
	ctx context.Context,
	owner, listID, productID uuid.UUID,
	quantity int64,
) (*domain.Demand, error) {
	demand, err := domain.NewDemand(
		domain.DemandData{ID: uuid.New(), ListID: listID, ProductID: productID, Required: quantity},
	)
	if err != nil {
		return nil, err
	}
	err = s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		list, err := p.List(ctx).Get(owner, listID)
		if err != nil {
			return err
		}
		if list == nil {
			return domain.ErrListNotFound
		}
		if err := p.Demand(ctx).CreateManual(demand); err != nil {
			return err
		}
		return s.recordDemandAudit(ctx, owner, demand, "shopping_demand.created")
	})
	if err != nil {
		return nil, fmt.Errorf("create manual demand: %w", err)
	}
	return demand, nil
}

func (s *Service) ListDemands(ctx context.Context, owner, listID uuid.UUID) ([]*domain.Demand, error) {
	var demands []*domain.Demand
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		list, err := p.List(ctx).Get(owner, listID)
		if err != nil {
			return err
		}
		if list == nil {
			return domain.ErrListNotFound
		}
		demands, err = p.Demand(ctx).List(listID)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list shopping demands: %w", err)
	}
	return demands, nil
}

func (s *Service) ConfirmPurchase(
	ctx context.Context,
	owner, listID, purchaseID, demandID, inventoryID uuid.UUID,
	quantity int64,
) error {
	if owner == uuid.Nil() || listID == uuid.Nil() || purchaseID == uuid.Nil() ||
		demandID == uuid.Nil() || inventoryID == uuid.Nil() || quantity <= 0 {
		return domain.ErrInvalidPurchase
	}
	var purchase *domain.Purchase
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		list, err := p.List(ctx).Get(owner, listID)
		if err != nil {
			return err
		}
		if list == nil {
			return domain.ErrListNotFound
		}
		demand, err := p.Demand(ctx).Lock(listID, demandID)
		if err != nil {
			return err
		}
		if demand == nil {
			return domain.ErrInvalidDemand
		}
		data := domain.PurchaseData{
			ID:          purchaseID,
			DemandID:    demandID,
			ProductID:   demand.Data().ProductID,
			InventoryID: inventoryID,
			Quantity:    quantity,
		}
		purchase, err = domain.NewPurchase(data)
		if err != nil {
			return err
		}
		created, err := p.Purchase(ctx).Create(listID, purchase)
		if err != nil || !created {
			return err
		}
		changed, err := demand.ApplyPurchase(purchase)
		if err != nil || !changed {
			return err
		}
		if err := p.Demand(ctx).Update(demand); err != nil {
			return err
		}
		if s.dispatcherFactory == nil {
			return errors.New("event dispatcher is not configured")
		}
		dispatcher := s.dispatcherFactory.NewDispatcher(ctx)
		event := &shoppingevents.PurchaseConfirmed{
			PurchaseId:     purchaseID.String(),
			ShoppingListId: listID.String(),
			DemandId:       demandID.String(),
			ProductId:      data.ProductID.String(),
			InventoryId:    inventoryID.String(),
			Quantity:       quantity,
			ActorId:        owner.String(),
		}
		if source := demand.Data().Source; source != nil {
			event.MealPlanId = source.MealPlanID.String()
			event.MealItemId = source.MealItemID.String()
			event.Revision = source.Revision.String()
		}
		if err := dispatcher.Dispatch(event); err != nil {
			return err
		}
		return s.recordDemandAudit(ctx, owner, demand, "shopping_purchase.confirmed")
	})
	if err != nil {
		return fmt.Errorf("confirm purchase: %w", err)
	}
	return nil
}

func (s *Service) recordDemandAudit(ctx context.Context, actor uuid.UUID, demand *domain.Demand, action string) error {
	if s.dispatcherFactory == nil {
		return errors.New("event dispatcher is not configured")
	}
	data := demand.Data()
	metadata, err := json.Marshal(map[string]any{
		"listId":    data.ListID.String(),
		"productId": data.ProductID.String(),
		"required":  data.Required,
		"purchased": data.Purchased,
	})
	if err != nil {
		return err
	}
	return s.dispatcherFactory.NewDispatcher(ctx).Dispatch(&auditevents.AuditRecordRequested{
		ActorId:      actor.String(),
		Action:       action,
		TargetType:   "shopping_demand",
		TargetId:     data.ID.String(),
		MetadataJson: metadata,
	})
}

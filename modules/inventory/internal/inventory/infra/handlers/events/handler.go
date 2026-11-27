package events

import (
	"context"
	"fmt"
	"log/slog"
	"uuid"

	inventoryevents "inventory/api/events"
	"inventory/internal/inventory/app"
	"inventory/internal/inventory/domain"
	mealplannerevents "mealplanner/api/events"
	shoppingevents "shopping/api/events"

	"github.com/distributed-programming-2026/go-sdk/pkg/event"
)

const mealPlannerProducer = "mealplanner"

type allocator interface {
	AllocateMeal(context.Context, app.MealAllocation) error
	CancelMealAllocation(context.Context, domain.AllocationIdentity) error
	CompleteMealAllocation(context.Context, domain.AllocationIdentity) error
	ReplenishStock(context.Context, uuid.UUID, uuid.UUID, uuid.UUID, uuid.UUID, int64) error
}
type handler struct {
	logger    *slog.Logger
	allocator allocator
}

func NewHandler(logger *slog.Logger, allocator allocator) event.Handler {
	return &handler{logger: logger, allocator: allocator}
}
func eventID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil() {
		return uuid.Nil(), fmt.Errorf("invalid event identifier %q", raw)
	}
	return id, nil
}
func (h *handler) Handle(ctx context.Context, envelope event.Envelope) error {
	if envelope.Producer() == "inventory" && envelope.Type() == "Echo" {
		var echo inventoryevents.Echo
		if err := envelope.UnmarshalPayload(&echo); err != nil {
			return err
		}
		h.logger.InfoContext(ctx, "echo received", slog.String("echo_id", echo.ID), slog.String("body", echo.Body))
		return nil
	}
	if envelope.Producer() == mealPlannerProducer && envelope.Type() == "MealCancellationRequested" {
		return h.cancel(ctx, envelope)
	}
	if envelope.Producer() == mealPlannerProducer && envelope.Type() == "MealCompletionRequested" {
		return h.complete(ctx, envelope)
	}
	if envelope.Producer() == "shopping" && envelope.Type() == "PurchaseConfirmed" {
		return h.replenish(ctx, envelope)
	}
	if envelope.Producer() != mealPlannerProducer || envelope.Type() != "MealPlanningRequested" {
		return fmt.Errorf("unsupported event %s.%s", envelope.Producer(), envelope.Type())
	}
	var payload mealplannerevents.MealPlanningRequested
	if err := envelope.UnmarshalPayload(&payload); err != nil {
		return err
	}
	plan, err := eventID(payload.PlanId)
	if err != nil {
		return err
	}
	item, err := eventID(payload.ItemId)
	if err != nil {
		return err
	}
	revision, err := eventID(payload.Revision)
	if err != nil {
		return err
	}
	actor, err := eventID(payload.ActorId)
	if err != nil {
		return err
	}
	shopping, err := eventID(payload.ShoppingListId)
	if err != nil {
		return err
	}
	request := app.MealAllocation{
		PlanID:         plan,
		ItemID:         item,
		Revision:       revision,
		ActorID:        actor,
		ShoppingListID: shopping,
	}
	for _, raw := range payload.InventoryIds {
		id, err := eventID(raw)
		if err != nil {
			return err
		}
		request.InventoryIDs = append(request.InventoryIDs, id)
	}
	for _, line := range payload.Ingredients {
		id, err := eventID(line.GetProductId())
		if err != nil {
			return err
		}
		request.Items = append(request.Items, domain.ReservationItem{ProductID: id, Quantity: line.GetQuantity()})
	}
	return h.allocator.AllocateMeal(ctx, request)
}

func (h *handler) replenish(ctx context.Context, envelope event.Envelope) error {
	var payload shoppingevents.PurchaseConfirmed
	if err := envelope.UnmarshalPayload(&payload); err != nil {
		return err
	}
	purchaseID, err := eventID(payload.PurchaseId)
	if err != nil {
		return err
	}
	inventoryID, err := eventID(payload.InventoryId)
	if err != nil {
		return err
	}
	productID, err := eventID(payload.ProductId)
	if err != nil {
		return err
	}
	actorID, err := eventID(payload.ActorId)
	if err != nil {
		return err
	}
	if payload.Quantity <= 0 {
		return fmt.Errorf("invalid purchase quantity")
	}
	return h.allocator.ReplenishStock(
		ctx,
		purchaseID,
		inventoryID,
		productID,
		actorID,
		payload.Quantity,
	)
}

func (h *handler) complete(ctx context.Context, envelope event.Envelope) error {
	var payload mealplannerevents.MealCompletionRequested
	if err := envelope.UnmarshalPayload(&payload); err != nil {
		return err
	}
	identity, err := allocationIdentity(
		payload.PlanId, payload.ItemId, payload.Revision, payload.ActorId, payload.ShoppingListId, payload.InventoryIds,
	)
	if err != nil {
		return err
	}
	return h.allocator.CompleteMealAllocation(ctx, identity)
}

func (h *handler) cancel(ctx context.Context, envelope event.Envelope) error {
	var payload mealplannerevents.MealCancellationRequested
	if err := envelope.UnmarshalPayload(&payload); err != nil {
		return err
	}
	identity, err := allocationIdentity(
		payload.PlanId, payload.ItemId, payload.Revision, payload.ActorId, payload.ShoppingListId, payload.InventoryIds,
	)
	if err != nil {
		return err
	}
	return h.allocator.CancelMealAllocation(ctx, identity)
}

func allocationIdentity(
	planRaw, itemRaw, revisionRaw, actorRaw, shoppingRaw string,
	inventoryRaw []string,
) (domain.AllocationIdentity, error) {
	plan, err := eventID(planRaw)
	if err != nil {
		return domain.AllocationIdentity{}, err
	}
	item, err := eventID(itemRaw)
	if err != nil {
		return domain.AllocationIdentity{}, err
	}
	revision, err := eventID(revisionRaw)
	if err != nil {
		return domain.AllocationIdentity{}, err
	}
	actor, err := eventID(actorRaw)
	if err != nil {
		return domain.AllocationIdentity{}, err
	}
	shopping, err := eventID(shoppingRaw)
	if err != nil {
		return domain.AllocationIdentity{}, err
	}
	identity := domain.AllocationIdentity{
		PlanID:         plan,
		ItemID:         item,
		Revision:       revision,
		ActorID:        actor,
		ShoppingListID: shopping,
	}
	for _, raw := range inventoryRaw {
		id, err := eventID(raw)
		if err != nil {
			return domain.AllocationIdentity{}, err
		}
		identity.InventoryIDs = append(identity.InventoryIDs, id)
	}
	return identity, nil
}

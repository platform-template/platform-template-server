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

	"github.com/distributed-programming-2026/go-sdk/pkg/event"
)

type allocator interface {
	AllocateMeal(context.Context, app.MealAllocation) error
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
	if envelope.Producer() != "mealplanner" || envelope.Type() != "MealPlanningRequested" {
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

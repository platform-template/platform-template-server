package events

import (
	"context"
	"fmt"
	"log/slog"
	"uuid"

	inventoryevents "inventory/api/events"
	shoppingevents "shopping/api/events"
	"shopping/internal/shopping/app"
	"shopping/internal/shopping/domain"

	"github.com/distributed-programming-2026/go-sdk/pkg/event"
)

type shortageRecorder interface {
	RecordShortages(context.Context, app.AllocationResult) error
}
type handler struct {
	logger  *slog.Logger
	service shortageRecorder
}

func NewHandler(logger *slog.Logger, service shortageRecorder) event.Handler {
	return &handler{logger: logger, service: service}
}
func parseID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil() {
		return uuid.Nil(), fmt.Errorf("invalid event identifier %q", raw)
	}
	return id, nil
}
func (h *handler) Handle(ctx context.Context, envelope event.Envelope) error {
	if envelope.Producer() == "shopping" && envelope.Type() == "Echo" {
		var echo shoppingevents.Echo
		if err := envelope.UnmarshalPayload(&echo); err != nil {
			return err
		}
		h.logger.InfoContext(ctx, "echo received", slog.String("echo_id", echo.ID), slog.String("body", echo.Body))
		return nil
	}
	if envelope.Producer() != "inventory" || envelope.Type() != "AllocationCompleted" {
		return fmt.Errorf("unsupported event %s.%s", envelope.Producer(), envelope.Type())
	}
	var payload inventoryevents.AllocationCompleted
	if err := envelope.UnmarshalPayload(&payload); err != nil {
		return err
	}
	actor, err := parseID(payload.ActorId)
	if err != nil {
		return err
	}
	list, err := parseID(payload.ShoppingListId)
	if err != nil {
		return err
	}
	plan, err := parseID(payload.PlanId)
	if err != nil {
		return err
	}
	item, err := parseID(payload.ItemId)
	if err != nil {
		return err
	}
	revision, err := parseID(payload.Revision)
	if err != nil {
		return err
	}
	result := app.AllocationResult{
		ActorID: actor,
		ListID:  list,
		Source:  domain.Source{MealPlanID: plan, MealItemID: item, Revision: revision},
		Version: payload.ResultVersion,
	}
	for _, shortage := range payload.Shortages {
		product, err := parseID(shortage.GetProductId())
		if err != nil {
			return err
		}
		result.Shortages = append(result.Shortages, app.Shortage{ProductID: product, Quantity: shortage.GetQuantity()})
	}
	return h.service.RecordShortages(ctx, result)
}

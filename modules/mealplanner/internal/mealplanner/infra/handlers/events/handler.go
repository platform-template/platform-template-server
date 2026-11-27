package events

import (
	"context"
	"fmt"
	"log/slog"
	"uuid"

	inventoryevents "inventory/api/events"
	mealplannerevents "mealplanner/api/events"
	"mealplanner/internal/mealplanner/app"
	"mealplanner/internal/mealplanner/domain"
	shoppingevents "shopping/api/events"

	"github.com/distributed-programming-2026/go-sdk/pkg/event"
)

type allocationHandler interface {
	ApplyAllocation(context.Context, app.AllocationResult) error
	ConfirmCancellation(context.Context, app.CancellationConfirmation) error
}
type handler struct {
	logger  *slog.Logger
	service allocationHandler
}

func NewHandler(logger *slog.Logger, service allocationHandler) event.Handler {
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
	if envelope.Producer() == "mealplanner" && envelope.Type() == "Echo" {
		var echo mealplannerevents.Echo
		if err := envelope.UnmarshalPayload(&echo); err != nil {
			return err
		}
		h.logger.InfoContext(ctx, "echo received", slog.String("echo_id", echo.ID), slog.String("body", echo.Body))
		return nil
	}
	if envelope.Producer() == "inventory" && envelope.Type() == "AllocationCancelled" {
		var payload inventoryevents.AllocationCancelled
		if err := envelope.UnmarshalPayload(&payload); err != nil {
			return err
		}
		return h.confirm(
			ctx,
			payload.ActorId,
			payload.PlanId,
			payload.ItemId,
			payload.Revision,
			payload.ShoppingListId,
			domain.InventoryParticipant,
		)
	}
	if envelope.Producer() == "shopping" && envelope.Type() == "ShortagesCancelled" {
		var payload shoppingevents.ShortagesCancelled
		if err := envelope.UnmarshalPayload(&payload); err != nil {
			return err
		}
		return h.confirm(
			ctx,
			payload.ActorId,
			payload.PlanId,
			payload.ItemId,
			payload.Revision,
			payload.ShoppingListId,
			domain.ShoppingParticipant,
		)
	}
	if envelope.Producer() != "inventory" || envelope.Type() != "AllocationCompleted" {
		return fmt.Errorf("unsupported event %s.%s", envelope.Producer(), envelope.Type())
	}
	var payload inventoryevents.AllocationCompleted
	if err := envelope.UnmarshalPayload(&payload); err != nil {
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
	actor, err := parseID(payload.ActorId)
	if err != nil {
		return err
	}
	shopping, err := parseID(payload.ShoppingListId)
	if err != nil {
		return err
	}
	result := app.AllocationResult{
		PlanID:         plan,
		ItemID:         item,
		Revision:       revision,
		ActorID:        actor,
		ShoppingListID: shopping,
		Version:        payload.ResultVersion,
	}
	for _, line := range payload.Allocations {
		inventory, err := parseID(line.GetInventoryId())
		if err != nil {
			return err
		}
		reservation, err := parseID(line.GetReservationId())
		if err != nil {
			return err
		}
		product, err := parseID(line.GetProductId())
		if err != nil {
			return err
		}
		result.Allocations = append(
			result.Allocations,
			domain.AllocationLine{
				InventoryID:   inventory,
				ReservationID: reservation,
				ProductID:     product,
				Quantity:      line.GetQuantity(),
			},
		)
	}
	for _, line := range payload.Shortages {
		product, err := parseID(line.GetProductId())
		if err != nil {
			return err
		}
		result.Shortages = append(result.Shortages, domain.Ingredient{ProductID: product, Quantity: line.GetQuantity()})
	}
	return h.service.ApplyAllocation(ctx, result)
}

func (h *handler) confirm(
	ctx context.Context,
	actorRaw, planRaw, itemRaw, revisionRaw, listRaw string,
	participant domain.CancellationParticipant,
) error {
	actor, err := parseID(actorRaw)
	if err != nil {
		return err
	}
	plan, err := parseID(planRaw)
	if err != nil {
		return err
	}
	item, err := parseID(itemRaw)
	if err != nil {
		return err
	}
	revision, err := parseID(revisionRaw)
	if err != nil {
		return err
	}
	list, err := parseID(listRaw)
	if err != nil {
		return err
	}
	return h.service.ConfirmCancellation(
		ctx,
		app.CancellationConfirmation{
			ActorID:        actor,
			PlanID:         plan,
			ItemID:         item,
			Revision:       revision,
			ShoppingListID: list,
			Participant:    participant,
		},
	)
}

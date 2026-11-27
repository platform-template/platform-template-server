package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"uuid"

	auditevents "audit/api/events"
	"audit/internal/audit/app"
	"audit/internal/audit/domain"
	inventoryevents "inventory/api/events"

	"github.com/distributed-programming-2026/go-sdk/pkg/event"
)

const (
	inventoryModule = "inventory"
	metadataName    = "name"
)

type recorder interface {
	RecordAuditEntry(context.Context, app.RecordAuditEntryInput) (*domain.AuditEntry, error)
}

type handler struct {
	logger   *slog.Logger
	recorder recorder
}

func NewHandler(logger *slog.Logger, recorder recorder) event.Handler {
	return &handler{logger: logger, recorder: recorder}
}

func (h *handler) Handle(ctx context.Context, envelope event.Envelope) error {
	if envelope.Producer() == "audit" && envelope.Type() == "Echo" {
		var echo auditevents.Echo
		if err := envelope.UnmarshalPayload(&echo); err != nil {
			return err
		}
		h.logger.InfoContext(ctx, "echo received", slog.String("echo_id", echo.ID), slog.String("body", echo.Body))
		return nil
	}
	if envelope.Producer() != inventoryModule {
		return fmt.Errorf("unsupported producer %q", envelope.Producer())
	}
	input, err := inventoryEntry(envelope)
	if err != nil {
		return err
	}
	_, err = h.recorder.RecordAuditEntry(ctx, input)
	return err
}

func inventoryEntry(envelope event.Envelope) (app.RecordAuditEntryInput, error) {
	input := app.RecordAuditEntryInput{
		EventID:       envelope.ID(),
		ModuleKey:     "inventory",
		CorrelationID: envelope.CorrelationID(),
		OccurredAt:    envelope.OccurredAt(),
	}
	var actor string
	var metadata any
	switch envelope.Type() {
	case "InventoryCreated":
		var payload inventoryevents.InventoryCreated
		if err := envelope.UnmarshalPayload(&payload); err != nil {
			return input, err
		}
		input.Action, input.TargetType, input.TargetID = "inventory.created", inventoryModule, payload.InventoryId
		actor, metadata = payload.ActorId, map[string]any{metadataName: payload.Name}
	case "InventoryRenamed":
		var payload inventoryevents.InventoryRenamed
		if err := envelope.UnmarshalPayload(&payload); err != nil {
			return input, err
		}
		input.Action, input.TargetType, input.TargetID = "inventory.renamed", inventoryModule, payload.InventoryId
		actor, metadata = payload.ActorId, map[string]any{metadataName: payload.Name}
	case "ProductCreated":
		var payload inventoryevents.ProductCreated
		if err := envelope.UnmarshalPayload(&payload); err != nil {
			return input, err
		}
		input.Action, input.TargetType, input.TargetID = "product.created", "product", payload.ProductId
		actor, metadata = payload.ActorId, map[string]any{metadataName: payload.Name, "unit": payload.Unit}
	case "ProductRenamed":
		var payload inventoryevents.ProductRenamed
		if err := envelope.UnmarshalPayload(&payload); err != nil {
			return input, err
		}
		input.Action, input.TargetType, input.TargetID = "product.renamed", "product", payload.ProductId
		actor, metadata = payload.ActorId, map[string]any{metadataName: payload.Name}
	case "StockCreated":
		var payload inventoryevents.StockCreated
		if err := envelope.UnmarshalPayload(&payload); err != nil {
			return input, err
		}
		if _, err := parseID(payload.ProductId); err != nil {
			return input, fmt.Errorf("invalid stock product: %w", err)
		}
		input.Action, input.TargetType, input.TargetID = "stock.created", inventoryModule, payload.InventoryId
		actor, metadata = payload.ActorId, map[string]any{"productId": payload.ProductId}
	case "StockAdjusted":
		var payload inventoryevents.StockAdjusted
		if err := envelope.UnmarshalPayload(&payload); err != nil {
			return input, err
		}
		if _, err := parseID(payload.ProductId); err != nil {
			return input, fmt.Errorf("invalid stock product: %w", err)
		}
		if payload.QuantityDelta == 0 {
			return input, errors.New("stock adjustment cannot be zero")
		}
		input.Action, input.TargetType, input.TargetID = "stock.adjusted", inventoryModule, payload.InventoryId
		actor, metadata = payload.ActorId, map[string]any{
			"productId":     payload.ProductId,
			"quantityDelta": payload.QuantityDelta,
		}
	default:
		return input, fmt.Errorf("unsupported inventory event type %q", envelope.Type())
	}
	if _, err := parseID(input.TargetID); err != nil {
		return input, fmt.Errorf("invalid event target: %w", err)
	}
	if actor != "" {
		id, err := parseID(actor)
		if err != nil {
			return input, fmt.Errorf("invalid event actor: %w", err)
		}
		input.ActorID = &id
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return input, fmt.Errorf("encode audit metadata: %w", err)
	}
	input.Metadata = encoded
	return input, nil
}

func parseID(value string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil(), err
	}
	if id == uuid.Nil() {
		return uuid.Nil(), fmt.Errorf("nil UUID")
	}
	return id, nil
}

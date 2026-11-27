package events

import (
	auditevents "audit/api/events"
	"audit/internal/audit/app"
	"audit/internal/audit/domain"
	"context"
	"fmt"
	"log/slog"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/event"
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
	if envelope.Type() != "AuditRecordRequested" {
		return fmt.Errorf("unsupported audit event type %q", envelope.Type())
	}
	var payload auditevents.AuditRecordRequested
	if err := envelope.UnmarshalPayload(&payload); err != nil {
		return err
	}
	input := app.RecordAuditEntryInput{EventID: envelope.ID(), ModuleKey: envelope.Producer(), Action: payload.Action,
		TargetType: payload.TargetType, TargetID: payload.TargetId, Metadata: payload.MetadataJson,
		CorrelationID: envelope.CorrelationID(), OccurredAt: envelope.OccurredAt()}
	if payload.ActorId != "" {
		id, err := uuid.Parse(payload.ActorId)
		if err != nil || id == uuid.Nil() {
			return fmt.Errorf("invalid audit actor ID %q", payload.ActorId)
		}
		input.ActorID = &id
	}
	_, err := h.recorder.RecordAuditEntry(ctx, input)
	return err
}

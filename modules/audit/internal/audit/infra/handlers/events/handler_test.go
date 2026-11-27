package events

import (
	auditevents "audit/api/events"
	"audit/internal/audit/app"
	"audit/internal/audit/domain"
	"context"
	"log/slog"
	"testing"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/event"
	"github.com/stretchr/testify/require"
)

type captureRecorder struct{ input app.RecordAuditEntryInput }

func (r *captureRecorder) RecordAuditEntry(
	_ context.Context,
	input app.RecordAuditEntryInput,
) (*domain.AuditEntry, error) {
	r.input = input
	return nil, nil
}
func TestGenericAuditEvent(t *testing.T) {
	for _, producer := range []string{"unknown-service", "another-service"} {
		actor := uuid.New()
		payload := &auditevents.AuditRecordRequested{
			ActorId:      actor.String(),
			Action:       "any.new.action",
			TargetType:   "arbitrary",
			TargetId:     "not-a-uuid",
			MetadataJson: []byte(`{"key":1}`),
		}
		envelope, err := event.New(t.Context(), producer, "AuditRecordRequested", payload)
		require.NoError(t, err)
		recorder := &captureRecorder{}
		require.NoError(t, NewHandler(slog.Default(), recorder).Handle(t.Context(), envelope))
		require.Equal(t, producer, recorder.input.ModuleKey)
		require.Equal(t, payload.Action, recorder.input.Action)
		require.Equal(t, payload.TargetId, recorder.input.TargetID)
		require.Equal(t, actor, *recorder.input.ActorID)
		require.Equal(t, envelope.ID(), recorder.input.EventID)
		require.Equal(t, envelope.CorrelationID(), recorder.input.CorrelationID)
	}
}

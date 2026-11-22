package domain

import (
	"encoding/json"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/require"
)

func TestAuditEntryValidation(t *testing.T) {
	valid := AuditEntryData{
		EventID:    "event-id",
		ModuleKey:  "auth",
		Action:     "user.registered",
		OccurredAt: time.Now(),
		Metadata:   json.RawMessage(`{"source":"web"}`),
	}
	tests := []struct {
		name   string
		change func(*AuditEntryData)
	}{
		{
			name: "missing event",
			change: func(data *AuditEntryData) {
				data.EventID = ""
			},
		},
		{
			name: "missing module",
			change: func(data *AuditEntryData) {
				data.ModuleKey = " "
			},
		},
		{
			name: "missing action",
			change: func(data *AuditEntryData) {
				data.Action = ""
			},
		},
		{
			name: "missing time",
			change: func(data *AuditEntryData) {
				data.OccurredAt = time.Time{}
			},
		},
		{
			name: "invalid JSON",
			change: func(data *AuditEntryData) {
				data.Metadata = []byte(`{`)
			},
		},
		{
			name: "zero actor",
			change: func(data *AuditEntryData) {
				actor := uuid.Nil()
				data.ActorID = &actor
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := valid
			test.change(&data)
			_, err := NewAuditEntry(data)
			require.ErrorIs(t, err, ErrInvalidAuditEntry)
		})
	}
	entry, err := NewAuditEntry(valid)
	require.NoError(t, err)
	require.Nil(t, entry.Data().ActorID)
	require.JSONEq(t, string(valid.Metadata), string(entry.Data().Metadata))
	require.False(t, entry.Data().RecordedAt.IsZero())
}

func TestAuditEntryImmutabilityAndTime(t *testing.T) {
	actor := uuid.New()
	metadata := json.RawMessage(`{"source":"web"}`)
	occurred := time.Date(2026, 1, 1, 12, 0, 0, 123456789, time.FixedZone("test", 3*60*60))
	entry, err := NewAuditEntry(AuditEntryData{
		EventID:    "event:1",
		ActorID:    &actor,
		ModuleKey:  "identity",
		Action:     "user.created",
		OccurredAt: occurred,
		Metadata:   metadata,
	})
	require.NoError(t, err)
	data := entry.Data()
	require.Equal(t, occurred.UTC(), data.OccurredAt)
	require.Equal(t, time.UTC, data.RecordedAt.Location())

	metadata[0] = 'x'
	actor = uuid.Nil()
	data.Metadata[0] = 'x'
	*data.ActorID = uuid.Nil()
	restored := entry.Data()
	require.JSONEq(t, `{"source":"web"}`, string(restored.Metadata))
	require.NotEqual(t, uuid.Nil(), *restored.ActorID)

	copyEntry, err := RestoreAuditEntry(restored)
	require.NoError(t, err)
	require.Equal(t, restored, copyEntry.Data())
	restored.Metadata[0] = 'x'
	*restored.ActorID = uuid.Nil()
	require.JSONEq(t, `{"source":"web"}`, string(copyEntry.Data().Metadata))
	require.NotEqual(t, uuid.Nil(), *copyEntry.Data().ActorID)
}

func TestRestoreAuditEntryRequiresRecordedAt(t *testing.T) {
	data := AuditEntryData{
		EventID:    "event-id",
		ModuleKey:  "auth",
		Action:     "login",
		OccurredAt: time.Now(),
	}
	_, err := RestoreAuditEntry(data)
	require.ErrorIs(t, err, ErrInvalidAuditEntry)

	data.RecordedAt = time.Now()
	entry, err := RestoreAuditEntry(data)
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(entry.Data().Metadata))
}

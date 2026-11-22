package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
	"uuid"
)

var (
	ErrInvalidAuditEntry   = errors.New("invalid audit entry")
	ErrAuditEntryDuplicate = errors.New("audit entry already exists")
	ErrInvalidAuditFilter  = errors.New("invalid audit filter")
)

type AuditEntryData struct {
	EventID       string
	ActorID       *uuid.UUID
	ModuleKey     string
	Action        string
	TargetType    string
	TargetID      string
	CorrelationID string
	Metadata      json.RawMessage
	OccurredAt    time.Time
	RecordedAt    time.Time
}

type AuditEntry struct{ data AuditEntryData }

func NewAuditEntry(data AuditEntryData) (*AuditEntry, error) {
	data.RecordedAt = time.Now().UTC()
	return RestoreAuditEntry(data)
}

func RestoreAuditEntry(data AuditEntryData) (*AuditEntry, error) {
	if strings.TrimSpace(data.EventID) == "" ||
		strings.TrimSpace(data.ModuleKey) == "" ||
		strings.TrimSpace(data.Action) == "" ||
		(data.ActorID != nil && *data.ActorID == uuid.Nil()) ||
		data.OccurredAt.IsZero() || data.RecordedAt.IsZero() {
		return nil, ErrInvalidAuditEntry
	}
	if len(data.Metadata) == 0 {
		data.Metadata = json.RawMessage(`{}`)
	}
	if !json.Valid(data.Metadata) {
		return nil, ErrInvalidAuditEntry
	}
	data.Metadata = append(json.RawMessage(nil), data.Metadata...)
	if data.ActorID != nil {
		actor := *data.ActorID
		data.ActorID = &actor
	}
	data.OccurredAt = data.OccurredAt.UTC()
	data.RecordedAt = data.RecordedAt.UTC()
	return &AuditEntry{data: data}, nil
}

func (entry *AuditEntry) Data() AuditEntryData {
	data := entry.data
	data.Metadata = append(json.RawMessage(nil), data.Metadata...)
	if data.ActorID != nil {
		actor := *data.ActorID
		data.ActorID = &actor
	}
	return data
}

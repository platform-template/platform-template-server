package domain

import (
	"time"
	"uuid"
)

type AuditFilter struct {
	ActorID    *uuid.UUID
	ModuleKey  string
	Action     string
	TargetType string
	TargetID   string
	From       *time.Time
	To         *time.Time
	Limit      int
	Offset     int
}

type AuditRepository interface {
	Append(*AuditEntry) error
	GetByEventID(string) (*AuditEntry, error)
	List(AuditFilter) ([]*AuditEntry, error)
}

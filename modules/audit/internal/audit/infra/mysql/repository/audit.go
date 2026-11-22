package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	driver "github.com/go-sql-driver/mysql"

	"audit/internal/audit/domain"
)

var ErrAuditStorageConstraint = errors.New("audit entry violates storage constraints")

type auditRepository struct {
	ctx    context.Context
	client mysql.ClientContext
}

func NewAuditRepository(ctx context.Context, client mysql.ClientContext) domain.AuditRepository {
	return &auditRepository{ctx: ctx, client: client}
}

func (r *auditRepository) Append(entry *domain.AuditEntry) error {
	data := entry.Data()
	if err := validateAuditStorage(data); err != nil {
		return err
	}
	var actorID any
	if data.ActorID != nil {
		actorID = data.ActorID[:]
	}
	_, err := r.client.ExecContext(r.ctx, "audit.append", `
INSERT INTO audit_entries (event_id, actor_id, module_key, action, target_type,
    target_id, correlation_id, metadata, occurred_at, recorded_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		data.EventID,
		actorID,
		data.ModuleKey,
		data.Action,
		data.TargetType,
		data.TargetID,
		data.CorrelationID,
		string(data.Metadata),
		data.OccurredAt.Truncate(time.Microsecond),
		data.RecordedAt.Truncate(time.Microsecond),
	)
	var mysqlError *driver.MySQLError
	if errors.As(err, &mysqlError) {
		switch mysqlError.Number {
		case 1062:
			return domain.ErrAuditEntryDuplicate
		case 1264, 1292, 1366, 1406, 3819:
			return ErrAuditStorageConstraint
		}
	}
	if err != nil {
		return fmt.Errorf("append audit entry: %w", err)
	}
	return nil
}

func validateAuditStorage(data domain.AuditEntryData) error {
	fields := []struct {
		value string
		limit int
	}{
		{data.EventID, 255},
		{data.ModuleKey, 128},
		{data.Action, 128},
		{data.TargetType, 128},
		{data.TargetID, 255},
		{data.CorrelationID, 255},
	}
	for _, field := range fields {
		if !utf8.ValidString(field.value) || utf8.RuneCountInString(field.value) > field.limit {
			return ErrAuditStorageConstraint
		}
	}
	if len(data.Metadata) > 16384 || !utf8.Valid(data.Metadata) {
		return ErrAuditStorageConstraint
	}
	for _, timestamp := range []time.Time{data.OccurredAt, data.RecordedAt} {
		if timestamp.Year() < 1000 || timestamp.Year() > 9999 {
			return ErrAuditStorageConstraint
		}
	}
	return nil
}

func (r *auditRepository) GetByEventID(eventID string) (*domain.AuditEntry, error) {
	row := r.client.QueryRowContext(r.ctx, "audit.get_by_event_id", `
SELECT event_id, actor_id, module_key, action, target_type, target_id,
    correlation_id, metadata, occurred_at, recorded_at
FROM audit_entries WHERE event_id = ?`, eventID)
	entry, err := scanAuditEntry(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get audit entry: %w", err)
	}
	return entry, nil
}

func (r *auditRepository) List(filter domain.AuditFilter) ([]*domain.AuditEntry, error) {
	if filter.Limit < 0 || filter.Limit > 100 || filter.Offset < 0 ||
		(filter.ActorID != nil && *filter.ActorID == uuid.Nil()) ||
		(filter.From != nil && filter.From.IsZero()) || (filter.To != nil && filter.To.IsZero()) ||
		(filter.From != nil && filter.To != nil && filter.From.After(*filter.To)) {
		return nil, domain.ErrInvalidAuditFilter
	}
	if filter.Limit == 0 {
		filter.Limit = 20
	}
	query := `SELECT event_id, actor_id, module_key, action, target_type, target_id,
    correlation_id, metadata, occurred_at, recorded_at FROM audit_entries WHERE 1=1`
	args := make([]any, 0, 8)
	if filter.ActorID != nil {
		query += ` AND actor_id = ?`
		args = append(args, filter.ActorID[:])
	}
	if filter.ModuleKey != "" {
		query += ` AND module_key = ?`
		args = append(args, filter.ModuleKey)
	}
	if filter.Action != "" {
		query += ` AND action = ?`
		args = append(args, filter.Action)
	}
	if filter.TargetType != "" {
		query += ` AND target_type = ?`
		args = append(args, filter.TargetType)
	}
	if filter.TargetID != "" {
		query += ` AND target_id = ?`
		args = append(args, filter.TargetID)
	}
	if filter.From != nil {
		query += ` AND occurred_at >= ?`
		args = append(args, filter.From.UTC())
	}
	if filter.To != nil {
		query += ` AND occurred_at < ?`
		args = append(args, filter.To.UTC())
	}
	query += ` ORDER BY occurred_at DESC, event_id DESC LIMIT ? OFFSET ?`
	args = append(args, filter.Limit, filter.Offset)
	rows, err := r.client.QueryContext(r.ctx, "audit.list", query, args...)
	if err != nil {
		return nil, fmt.Errorf("list audit entries: %w", err)
	}
	defer rows.Close()
	entries := make([]*domain.AuditEntry, 0)
	for rows.Next() {
		entry, err := scanAuditEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("scan audit entry: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit entries: %w", err)
	}
	return entries, nil
}

type rowScanner interface{ Scan(...any) error }

func scanAuditEntry(row rowScanner) (*domain.AuditEntry, error) {
	var data domain.AuditEntryData
	var actor []byte
	var metadata string
	var occurredAt, recordedAt time.Time
	err := row.Scan(
		&data.EventID,
		&actor,
		&data.ModuleKey,
		&data.Action,
		&data.TargetType,
		&data.TargetID,
		&data.CorrelationID,
		&metadata,
		&occurredAt,
		&recordedAt,
	)
	if err != nil {
		return nil, err
	}
	if actor != nil {
		if len(actor) != 16 {
			return nil, errors.New("invalid binary actor ID")
		}
		var actorID uuid.UUID
		copy(actorID[:], actor)
		data.ActorID = &actorID
	}
	data.Metadata = []byte(metadata)
	data.OccurredAt = occurredAt
	data.RecordedAt = recordedAt
	entry, err := domain.RestoreAuditEntry(data)
	if err != nil {
		return nil, fmt.Errorf("restore audit entry: %w", err)
	}
	return entry, nil
}

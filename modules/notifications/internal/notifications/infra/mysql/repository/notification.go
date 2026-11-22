package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	driver "github.com/go-sql-driver/mysql"

	"notifications/internal/notifications/domain"
)

type repository struct {
	ctx    context.Context
	client mysql.ClientContext
}

func New(ctx context.Context, client mysql.ClientContext) domain.NotificationRepository {
	return &repository{ctx: ctx, client: client}
}

func (r *repository) Create(notification *domain.Notification) error {
	data := notification.Data()
	_, err := r.client.ExecContext(r.ctx, "notifications.create", `
INSERT INTO notifications (id, user_id, kind, title, body, created_at, read_at, idempotency_key)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		data.ID[:],
		data.UserID[:],
		string(data.Kind),
		data.Title,
		data.Body,
		data.CreatedAt,
		data.ReadAt,
		data.IdempotencyKey,
	)
	var mysqlErr *driver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return fmt.Errorf("%w: %v", domain.ErrDuplicate, err)
	}
	if err != nil {
		return fmt.Errorf("create notification: %w", err)
	}
	return nil
}

func (r *repository) GetByID(userID, id uuid.UUID) (*domain.Notification, error) {
	row := r.client.QueryRowContext(r.ctx, "notifications.get", `
SELECT id, user_id, kind, title, body, created_at, read_at, idempotency_key
FROM notifications WHERE user_id = ? AND id = ?`, userID[:], id[:])

	notification, err := scan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get notification: %w", err)
	}
	return notification, nil
}

func (r *repository) ListByUser(userID uuid.UUID, options domain.ListOptions) ([]*domain.Notification, error) {
	if options.Limit <= 0 || options.Limit > 1000 || options.Offset < 0 {
		return nil, domain.ErrInvalidNotification
	}

	query := `SELECT id, user_id, kind, title, body, created_at, read_at, idempotency_key
FROM notifications WHERE user_id = ?`
	if options.UnreadOnly {
		query += ` AND read_at IS NULL`
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`

	rows, err := r.client.QueryContext(
		r.ctx,
		"notifications.list",
		query,
		userID[:],
		options.Limit,
		options.Offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	defer rows.Close()

	notifications := make([]*domain.Notification, 0)
	for rows.Next() {
		notification, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("scan notification: %w", err)
		}
		notifications = append(notifications, notification)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate notifications: %w", err)
	}
	return notifications, nil
}

func (r *repository) CountUnread(userID uuid.UUID) (int64, error) {
	var count int64
	row := r.client.QueryRowContext(r.ctx, "notifications.count_unread", `
SELECT COUNT(*) FROM notifications WHERE user_id = ? AND read_at IS NULL`, userID[:])
	if err := row.Scan(&count); err != nil {
		return 0, fmt.Errorf("count unread notifications: %w", err)
	}
	return count, nil
}

func (r *repository) MarkAsRead(userID, id uuid.UUID, at time.Time) error {
	if at.IsZero() || at.UTC().Year() < 1000 || at.UTC().Year() > 9999 {
		return domain.ErrInvalidReadAt
	}
	at = at.UTC().Truncate(time.Microsecond)

	result, err := r.client.ExecContext(r.ctx, "notifications.mark_read", `
UPDATE notifications SET read_at = ?
WHERE user_id = ? AND id = ? AND read_at IS NULL AND created_at <= ?`,
		at,
		userID[:],
		id[:],
		at,
	)
	if err != nil {
		return fmt.Errorf("mark notification read: %w", err)
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count updated notifications: %w", err)
	}
	if changed != 0 {
		return nil
	}

	notification, err := r.GetByID(userID, id)
	if err != nil {
		return err
	}
	if notification == nil {
		return domain.ErrNotFound
	}
	if notification.Data().ReadAt == nil {
		return domain.ErrInvalidReadAt
	}
	return nil
}

type scanner interface{ Scan(...any) error }

func scan(row scanner) (*domain.Notification, error) {
	var idBytes, userIDBytes []byte
	var kind, title, body string
	var created time.Time
	var read sql.NullTime
	var key sql.NullString

	if err := row.Scan(&idBytes, &userIDBytes, &kind, &title, &body, &created, &read, &key); err != nil {
		return nil, err
	}
	if len(idBytes) != 16 || len(userIDBytes) != 16 {
		return nil, errors.New("invalid binary notification UUID")
	}
	var id, userID uuid.UUID
	copy(id[:], idBytes)
	copy(userID[:], userIDBytes)

	data := domain.NotificationData{
		ID:        id,
		UserID:    userID,
		Kind:      domain.NotificationKind(kind),
		Title:     title,
		Body:      body,
		CreatedAt: created,
	}
	if read.Valid {
		data.ReadAt = &read.Time
	}
	if key.Valid {
		data.IdempotencyKey = &key.String
	}

	notification, err := domain.RestoreNotification(data)
	if err != nil {
		return nil, fmt.Errorf("restore notification: %w", err)
	}
	return notification, nil
}

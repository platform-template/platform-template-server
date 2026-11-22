package domain

import (
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
	"uuid"
)

const (
	MaxKindLength           = 128
	MaxTitleLength          = 255
	MaxBodyLength           = 10000
	MaxIdempotencyKeyLength = 255
)

var (
	ErrInvalidNotification = errors.New("invalid notification")
	ErrInvalidKind         = errors.New("invalid notification kind")
	ErrInvalidReadAt       = errors.New("invalid read time")
	ErrDuplicate           = errors.New("notification already exists")
	ErrNotFound            = errors.New("notification not found")
)

type NotificationKind string

var kindPattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)*$`)

func NewNotificationKind(raw string) (NotificationKind, error) {
	value := strings.ToLower(strings.TrimSpace(raw))
	if len(value) > MaxKindLength || !kindPattern.MatchString(value) {
		return "", ErrInvalidKind
	}
	return NotificationKind(value), nil
}

type NotificationData struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	Kind           NotificationKind
	Title          string
	Body           string
	CreatedAt      time.Time
	ReadAt         *time.Time
	IdempotencyKey *string
}

type Notification struct{ data NotificationData }

func NewNotification(data NotificationData) (*Notification, error) {
	if data.ReadAt != nil {
		return nil, ErrInvalidReadAt
	}
	return RestoreNotification(data)
}

func RestoreNotification(data NotificationData) (*Notification, error) {
	kind, err := NewNotificationKind(string(data.Kind))
	if err != nil {
		return nil, err
	}
	if data.ID == uuid.Nil() || data.UserID == uuid.Nil() || data.CreatedAt.IsZero() ||
		data.CreatedAt.UTC().Year() < 1000 ||
		data.CreatedAt.UTC().Year() > 9999 ||
		strings.TrimSpace(data.Title) == "" ||
		!validText(data.Title, MaxTitleLength) ||
		!validText(data.Body, MaxBodyLength) {
		return nil, ErrInvalidNotification
	}
	if data.IdempotencyKey != nil &&
		(strings.TrimSpace(*data.IdempotencyKey) == "" || !validText(*data.IdempotencyKey, MaxIdempotencyKeyLength)) {
		return nil, ErrInvalidNotification
	}
	data.Kind = kind
	if data.ReadAt != nil &&
		(data.ReadAt.IsZero() || data.ReadAt.Before(data.CreatedAt) || data.ReadAt.UTC().Year() > 9999) {
		return nil, ErrInvalidReadAt
	}
	data.CreatedAt = data.CreatedAt.UTC().Truncate(time.Microsecond)
	if data.ReadAt != nil {
		readAt := data.ReadAt.UTC().Truncate(time.Microsecond)
		if readAt.Before(data.CreatedAt) || readAt.Year() > 9999 {
			return nil, ErrInvalidReadAt
		}
		data.ReadAt = &readAt
	}
	if data.IdempotencyKey != nil {
		key := *data.IdempotencyKey
		data.IdempotencyKey = &key
	}
	return &Notification{data: data}, nil
}

func validText(value string, limit int) bool {
	return utf8.ValidString(value) && utf8.RuneCountInString(value) <= limit
}

func (n *Notification) Data() NotificationData {
	data := n.data
	if data.ReadAt != nil {
		value := *data.ReadAt
		data.ReadAt = &value
	}
	if data.IdempotencyKey != nil {
		value := *data.IdempotencyKey
		data.IdempotencyKey = &value
	}
	return data
}

func (n *Notification) MarkAsRead(at time.Time) error {
	if n.data.ReadAt != nil {
		return nil
	}
	if at.IsZero() || at.Before(n.data.CreatedAt) || at.UTC().Year() > 9999 {
		return ErrInvalidReadAt
	}
	value := at.UTC().Truncate(time.Microsecond)
	n.data.ReadAt = &value
	return nil
}

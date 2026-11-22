package domain

import (
	"time"
	"uuid"
)

type ListOptions struct {
	Limit      int
	Offset     int
	UnreadOnly bool
}

type NotificationRepository interface {
	Create(*Notification) error
	GetByID(uuid.UUID, uuid.UUID) (*Notification, error)
	ListByUser(uuid.UUID, ListOptions) ([]*Notification, error)
	CountUnread(uuid.UUID) (int64, error)
	MarkAsRead(uuid.UUID, uuid.UUID, time.Time) error
}

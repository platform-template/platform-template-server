package app

import (
	"context"
	"fmt"
	"time"
	"uuid"

	"notifications/internal/notifications/domain"
	"notifications/internal/notifications/infra/mysql"
)

const DefaultLimit = 20

type CreateNotificationInput struct {
	UserID         uuid.UUID
	Kind           string
	Title          string
	Body           string
	IdempotencyKey *string
}

func (s *Service) CreateNotification(ctx context.Context, input CreateNotificationInput) (*domain.Notification, error) {
	notification, err := domain.NewNotification(domain.NotificationData{
		ID:             uuid.New(),
		UserID:         input.UserID,
		Kind:           domain.NotificationKind(input.Kind),
		Title:          input.Title,
		Body:           input.Body,
		CreatedAt:      time.Now().UTC(),
		IdempotencyKey: input.IdempotencyKey,
	})
	if err != nil {
		return nil, err
	}
	err = s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		return provider.Notification(ctx).Create(notification)
	})
	if err != nil {
		return nil, fmt.Errorf("create notification: %w", err)
	}
	return notification, nil
}

func (s *Service) ListNotifications(
	ctx context.Context,
	userID uuid.UUID,
	options domain.ListOptions,
) ([]*domain.Notification, error) {
	if userID == uuid.Nil() || options.Limit < 0 || options.Limit > 1000 || options.Offset < 0 {
		return nil, domain.ErrInvalidNotification
	}
	if options.Limit == 0 {
		options.Limit = DefaultLimit
	}
	var notifications []*domain.Notification
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		var err error
		notifications, err = provider.Notification(ctx).ListByUser(userID, options)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list notifications: %w", err)
	}
	if notifications == nil {
		notifications = make([]*domain.Notification, 0)
	}
	return notifications, nil
}

func (s *Service) GetUnreadCount(ctx context.Context, userID uuid.UUID) (int64, error) {
	if userID == uuid.Nil() {
		return 0, domain.ErrInvalidNotification
	}
	var count int64
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		var err error
		count, err = provider.Notification(ctx).CountUnread(userID)
		return err
	})
	if err != nil {
		return 0, fmt.Errorf("count unread notifications: %w", err)
	}
	return count, nil
}

func (s *Service) MarkAsRead(ctx context.Context, userID, notificationID uuid.UUID) error {
	if userID == uuid.Nil() || notificationID == uuid.Nil() {
		return domain.ErrInvalidNotification
	}
	readAt := time.Now().UTC()
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		return provider.Notification(ctx).MarkAsRead(userID, notificationID, readAt)
	})
	if err != nil {
		return fmt.Errorf("mark notification read: %w", err)
	}
	return nil
}

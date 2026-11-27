package events

import (
	"context"
	"log/slog"
	"testing"
	"uuid"

	notificationsevents "notifications/api/events"
	"notifications/internal/notifications/app"
	"notifications/internal/notifications/domain"

	"github.com/distributed-programming-2026/go-sdk/pkg/event"
	"github.com/stretchr/testify/require"
)

type captureCreator struct {
	input app.CreateNotificationInput
	calls int
}

func (c *captureCreator) CreateNotification(
	_ context.Context,
	input app.CreateNotificationInput,
) (*domain.Notification, error) {
	c.input = input
	c.calls++
	return nil, nil
}

func TestNotificationRequest(t *testing.T) {
	creator := &captureCreator{}
	userID := uuid.New()
	envelope, err := event.New(
		t.Context(),
		"new-service",
		"NotificationRequested",
		&notificationsevents.NotificationRequested{
			UserId:         userID.String(),
			Kind:           "anything.happened",
			Title:          "Title",
			Body:           "Body",
			IdempotencyKey: "meal:ready",
		},
	)
	require.NoError(t, err)
	require.NoError(t, NewHandler(slog.Default(), creator).Handle(t.Context(), envelope))
	require.Equal(
		t,
		app.CreateNotificationInput{
			UserID:         userID,
			Kind:           "anything.happened",
			Title:          "Title",
			Body:           "Body",
			IdempotencyKey: stringPtr("meal:ready"),
		},
		creator.input,
	)
	invalid, err := event.New(
		t.Context(),
		"new-service",
		"NotificationRequested",
		&notificationsevents.NotificationRequested{UserId: "invalid"},
	)
	require.NoError(t, err)
	require.Error(t, NewHandler(slog.Default(), creator).Handle(t.Context(), invalid))
	require.Equal(t, 1, creator.calls)
}

func stringPtr(value string) *string { return &value }

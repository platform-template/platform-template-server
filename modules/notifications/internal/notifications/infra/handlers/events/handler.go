package events

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	notificationsevents "notifications/api/events"
	"notifications/internal/notifications/app"
	"notifications/internal/notifications/domain"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/event"
)

type creator interface {
	CreateNotification(context.Context, app.CreateNotificationInput) (*domain.Notification, error)
}
type handler struct {
	logger  *slog.Logger
	creator creator
}

func NewHandler(logger *slog.Logger, creator creator) event.Handler {
	return &handler{logger: logger, creator: creator}
}
func (h *handler) Handle(ctx context.Context, envelope event.Envelope) error {
	if envelope.Producer() == "notifications" && envelope.Type() == "Echo" {
		var echo notificationsevents.Echo
		if err := envelope.UnmarshalPayload(&echo); err != nil {
			return err
		}
		h.logger.InfoContext(ctx, "echo received", slog.String("echo_id", echo.ID), slog.String("body", echo.Body))
		return nil
	}
	if envelope.Type() != "NotificationRequested" {
		return fmt.Errorf("unsupported event type %q", envelope.Type())
	}
	var payload notificationsevents.NotificationRequested
	if err := envelope.UnmarshalPayload(&payload); err != nil {
		return err
	}
	userID, err := uuid.Parse(payload.UserId)
	if err != nil || userID == uuid.Nil() {
		return fmt.Errorf("invalid notification user ID %q", payload.UserId)
	}
	input := app.CreateNotificationInput{UserID: userID, Kind: payload.Kind, Title: payload.Title, Body: payload.Body}
	if payload.IdempotencyKey != "" {
		key := payload.IdempotencyKey
		input.IdempotencyKey = &key
	}
	_, err = h.creator.CreateNotification(
		ctx,
		input,
	)
	if errors.Is(err, domain.ErrDuplicate) {
		return nil
	}
	return err
}

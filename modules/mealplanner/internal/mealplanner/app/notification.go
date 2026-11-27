package app

import (
	"context"

	notificationsevents "notifications/api/events"
	"uuid"
)

// notify emits the universal Notifications contract. MealPlanner owns the choice
// of user-visible transitions; Notifications does not interpret meal semantics.
func (s *Service) notify(ctx context.Context, user uuid.UUID, kind, title, body, key string) error {
	if s.dispatcherFactory == nil {
		return nil
	}
	return s.dispatcherFactory.NewDispatcher(ctx).Dispatch(&notificationsevents.NotificationRequested{
		UserId:         user.String(),
		Kind:           kind,
		Title:          title,
		Body:           body,
		IdempotencyKey: key,
	})
}

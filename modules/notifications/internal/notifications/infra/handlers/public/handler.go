package public

import (
	"context"
	"errors"
	"uuid"

	publicapi "notifications/api/server/publicapi"
	"notifications/internal/notifications/app"
	"notifications/internal/notifications/domain"
)

type handler struct {
	svc *app.Service
}

func NewHandler(svc *app.Service) publicapi.StrictServerInterface {
	return &handler{
		svc: svc,
	}
}

func (h handler) ListNotifications(
	ctx context.Context,
	req publicapi.ListNotificationsRequestObject,
) (publicapi.ListNotificationsResponseObject, error) {
	userID, ok := ctx.Value(userIDKey{}).(uuid.UUID)
	if !ok || userID == uuid.Nil() {
		return publicapi.ListNotifications401Response{}, nil
	}
	options := domain.ListOptions{Limit: app.DefaultLimit}
	if req.Params.Limit != nil {
		options.Limit = *req.Params.Limit
	}
	if req.Params.Offset != nil {
		options.Offset = *req.Params.Offset
	}
	if req.Params.UnreadOnly != nil {
		options.UnreadOnly = *req.Params.UnreadOnly
	}
	if options.Limit <= 0 || options.Limit > 1000 || options.Offset < 0 {
		return publicapi.ListNotifications400Response{}, nil
	}
	notifications, err := h.svc.ListNotifications(ctx, userID, options)
	if errors.Is(err, domain.ErrInvalidNotification) {
		return publicapi.ListNotifications400Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	items := make([]publicapi.Notification, 0, len(notifications))
	for _, notification := range notifications {
		data := notification.Data()
		items = append(items, publicapi.Notification{
			Id:        data.ID.String(),
			Kind:      string(data.Kind),
			Title:     data.Title,
			Body:      data.Body,
			CreatedAt: data.CreatedAt.UTC(),
			ReadAt:    data.ReadAt,
		})
	}
	return publicapi.ListNotifications200JSONResponse{Notifications: items}, nil
}

func (h handler) GetUnreadCount(
	ctx context.Context,
	_ publicapi.GetUnreadCountRequestObject,
) (publicapi.GetUnreadCountResponseObject, error) {
	userID, ok := ctx.Value(userIDKey{}).(uuid.UUID)
	if !ok || userID == uuid.Nil() {
		return publicapi.GetUnreadCount401Response{}, nil
	}
	count, err := h.svc.GetUnreadCount(ctx, userID)
	if err != nil {
		return nil, err
	}
	return publicapi.GetUnreadCount200JSONResponse{Count: count}, nil
}

func (h handler) MarkNotificationRead(
	ctx context.Context,
	req publicapi.MarkNotificationReadRequestObject,
) (publicapi.MarkNotificationReadResponseObject, error) {
	userID, ok := ctx.Value(userIDKey{}).(uuid.UUID)
	if !ok || userID == uuid.Nil() {
		return publicapi.MarkNotificationRead401Response{}, nil
	}
	notificationID, err := uuid.Parse(req.Id)
	if err != nil || notificationID == uuid.Nil() {
		return publicapi.MarkNotificationRead400Response{}, nil
	}
	err = h.svc.MarkAsRead(ctx, userID, notificationID)
	if errors.Is(err, domain.ErrNotFound) {
		return publicapi.MarkNotificationRead404Response{}, nil
	}
	if errors.Is(err, domain.ErrInvalidNotification) {
		return publicapi.MarkNotificationRead400Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.MarkNotificationRead204Response{}, nil
}

func (h handler) Echo(ctx context.Context, req publicapi.EchoRequestObject) (publicapi.EchoResponseObject, error) {
	id, err := h.svc.Echo(ctx, req.Body.Body)
	if err != nil {
		return nil, err
	}

	return publicapi.Echo200JSONResponse{
		OperationID: id.String(),
	}, nil
}

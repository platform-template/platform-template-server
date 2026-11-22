package internalapi

import (
	"context"
	"log/slog"

	"github.com/distributed-programming-2026/go-sdk/pkg/logging"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	notificationsinternalapi "notifications/api/server/internalapi"
	"notifications/internal/notifications/app"
)

func NewHandler(svc *app.Service, logger *slog.Logger) notificationsinternalapi.NotificationsServiceServer {
	return &handler{
		svc:    svc,
		logger: logger,
	}
}

type handler struct {
	svc    *app.Service
	logger *slog.Logger
	notificationsinternalapi.UnimplementedNotificationsServiceServer
}

func (h handler) Echo(
	ctx context.Context,
	req *notificationsinternalapi.EchoRequest,
) (*notificationsinternalapi.EchoResponse, error) {
	id, err := h.svc.Echo(ctx, req.Body)
	if err != nil {
		h.logger.ErrorContext(ctx, "echo failed", logging.Error(err))
		return nil, status.Error(codes.Internal, "echo failed")
	}

	return &notificationsinternalapi.EchoResponse{
		OperationID: id.String(),
	}, nil
}

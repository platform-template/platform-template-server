package internalapi

import (
	"context"
	"log/slog"

	"github.com/distributed-programming-2026/go-sdk/pkg/logging"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sessioninternalapi "session/api/server/internalapi"
	"session/internal/session/app"
)

func NewHandler(svc *app.Service, logger *slog.Logger) sessioninternalapi.SessionServiceServer {
	return &handler{
		svc:    svc,
		logger: logger,
	}
}

type handler struct {
	svc    *app.Service
	logger *slog.Logger
	sessioninternalapi.UnimplementedSessionServiceServer
}

func (h handler) Echo(ctx context.Context, req *sessioninternalapi.EchoRequest) (*sessioninternalapi.EchoResponse, error) {
	id, err := h.svc.Echo(ctx, req.Body)
	if err != nil {
		h.logger.ErrorContext(ctx, "echo failed", logging.Error(err))
		return nil, status.Error(codes.Internal, "echo failed")
	}

	return &sessioninternalapi.EchoResponse{
		OperationID: id.String(),
	}, nil
}

package internalapi

import (
	"context"
	"log/slog"

	"github.com/distributed-programming-2026/go-sdk/pkg/logging"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authinternalapi "auth/api/server/internalapi"
	"auth/internal/auth/app"
)

func NewHandler(svc *app.Service, logger *slog.Logger) authinternalapi.AuthServiceServer {
	return &handler{
		svc:    svc,
		logger: logger,
	}
}

type handler struct {
	svc    *app.Service
	logger *slog.Logger
	authinternalapi.UnimplementedAuthServiceServer
}

func (h handler) Echo(ctx context.Context, req *authinternalapi.EchoRequest) (*authinternalapi.EchoResponse, error) {
	id, err := h.svc.Echo(ctx, req.Body)
	if err != nil {
		h.logger.ErrorContext(ctx, "echo failed", logging.Error(err))
		return nil, status.Error(codes.Internal, "echo failed")
	}

	return &authinternalapi.EchoResponse{
		OperationID: id.String(),
	}, nil
}

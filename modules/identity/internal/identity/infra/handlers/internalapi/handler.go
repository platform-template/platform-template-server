package internalapi

import (
	"context"
	"log/slog"

	"github.com/distributed-programming-2026/go-sdk/pkg/logging"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityinternalapi "identity/api/server/internalapi"
	"identity/internal/identity/app"
)

func NewHandler(svc *app.Service, logger *slog.Logger) identityinternalapi.IdentityServiceServer {
	return &handler{
		svc:    svc,
		logger: logger,
	}
}

type handler struct {
	svc    *app.Service
	logger *slog.Logger
	identityinternalapi.UnimplementedIdentityServiceServer
}

func (h handler) Echo(ctx context.Context, req *identityinternalapi.EchoRequest) (*identityinternalapi.EchoResponse, error) {
	id, err := h.svc.Echo(ctx, req.Body)
	if err != nil {
		h.logger.ErrorContext(ctx, "echo failed", logging.Error(err))
		return nil, status.Error(codes.Internal, "echo failed")
	}

	return &identityinternalapi.EchoResponse{
		OperationID: id.String(),
	}, nil
}

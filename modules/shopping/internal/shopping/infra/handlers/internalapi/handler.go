package internalapi

import (
	"context"
	"log/slog"

	"github.com/distributed-programming-2026/go-sdk/pkg/logging"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	shoppinginternalapi "shopping/api/server/internalapi"
	"shopping/internal/shopping/app"
)

func NewHandler(svc *app.Service, logger *slog.Logger) shoppinginternalapi.ShoppingServiceServer {
	return &handler{
		svc:    svc,
		logger: logger,
	}
}

type handler struct {
	svc    *app.Service
	logger *slog.Logger
	shoppinginternalapi.UnimplementedShoppingServiceServer
}

func (h handler) Echo(
	ctx context.Context,
	req *shoppinginternalapi.EchoRequest,
) (*shoppinginternalapi.EchoResponse, error) {
	id, err := h.svc.Echo(ctx, req.Body)
	if err != nil {
		h.logger.ErrorContext(ctx, "echo failed", logging.Error(err))
		return nil, status.Error(codes.Internal, "echo failed")
	}

	return &shoppinginternalapi.EchoResponse{
		OperationID: id.String(),
	}, nil
}

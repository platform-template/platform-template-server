package internalapi

import (
	"context"
	"log/slog"

	"github.com/distributed-programming-2026/go-sdk/pkg/logging"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	inventoryinternalapi "inventory/api/server/internalapi"
	"inventory/internal/inventory/app"
)

func NewHandler(svc *app.Service, logger *slog.Logger) inventoryinternalapi.InventoryServiceServer {
	return &handler{
		svc:    svc,
		logger: logger,
	}
}

type handler struct {
	svc    *app.Service
	logger *slog.Logger
	inventoryinternalapi.UnimplementedInventoryServiceServer
}

func (h handler) Echo(ctx context.Context, req *inventoryinternalapi.EchoRequest) (*inventoryinternalapi.EchoResponse, error) {
	id, err := h.svc.Echo(ctx, req.Body)
	if err != nil {
		h.logger.ErrorContext(ctx, "echo failed", logging.Error(err))
		return nil, status.Error(codes.Internal, "echo failed")
	}

	return &inventoryinternalapi.EchoResponse{
		OperationID: id.String(),
	}, nil
}

package internalapi

import (
	"context"
	"log/slog"

	"github.com/distributed-programming-2026/go-sdk/pkg/logging"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	recipesinternalapi "recipes/api/server/internalapi"
	"recipes/internal/recipes/app"
)

func NewHandler(svc *app.Service, logger *slog.Logger) recipesinternalapi.RecipesServiceServer {
	return &handler{
		svc:    svc,
		logger: logger,
	}
}

type handler struct {
	svc    *app.Service
	logger *slog.Logger
	recipesinternalapi.UnimplementedRecipesServiceServer
}

func (h handler) Echo(ctx context.Context, req *recipesinternalapi.EchoRequest) (*recipesinternalapi.EchoResponse, error) {
	id, err := h.svc.Echo(ctx, req.Body)
	if err != nil {
		h.logger.ErrorContext(ctx, "echo failed", logging.Error(err))
		return nil, status.Error(codes.Internal, "echo failed")
	}

	return &recipesinternalapi.EchoResponse{
		OperationID: id.String(),
	}, nil
}

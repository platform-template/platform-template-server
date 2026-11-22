package internalapi

import (
	"context"
	"log/slog"

	"github.com/distributed-programming-2026/go-sdk/pkg/logging"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	auditinternalapi "audit/api/server/internalapi"
	"audit/internal/audit/app"
)

func NewHandler(svc *app.Service, logger *slog.Logger) auditinternalapi.AuditServiceServer {
	return &handler{
		svc:    svc,
		logger: logger,
	}
}

type handler struct {
	svc    *app.Service
	logger *slog.Logger
	auditinternalapi.UnimplementedAuditServiceServer
}

func (h handler) Echo(ctx context.Context, req *auditinternalapi.EchoRequest) (*auditinternalapi.EchoResponse, error) {
	id, err := h.svc.Echo(ctx, req.Body)
	if err != nil {
		h.logger.ErrorContext(ctx, "echo failed", logging.Error(err))
		return nil, status.Error(codes.Internal, "echo failed")
	}

	return &auditinternalapi.EchoResponse{
		OperationID: id.String(),
	}, nil
}

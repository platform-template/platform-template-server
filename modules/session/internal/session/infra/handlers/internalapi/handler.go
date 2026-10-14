package internalapi

import (
	"context"
	"errors"
	"log/slog"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/logging"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sessioninternalapi "session/api/server/internalapi"
	"session/internal/session/app"
	"session/internal/session/domain"
)

func NewHandler(svc *app.Service, logger *slog.Logger) sessioninternalapi.SessionServiceServer {
	return &handler{svc: svc, logger: logger}
}

type handler struct {
	svc    *app.Service
	logger *slog.Logger
	sessioninternalapi.UnimplementedSessionServiceServer
}

func (h handler) CreateSession(ctx context.Context, req *sessioninternalapi.CreateSessionRequest) (*sessioninternalapi.CreateSessionResponse, error) {
	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user ID")
	}

	token, err := h.svc.Create(ctx, userID)
	if err != nil {
		return nil, h.handleError(ctx, "create session", err)
	}

	return &sessioninternalapi.CreateSessionResponse{
		Token: token,
	}, nil
}

func (h handler) ResolveSession(ctx context.Context, req *sessioninternalapi.ResolveSessionRequest) (*sessioninternalapi.ResolveSessionResponse, error) {
	userID, err := h.svc.Resolve(ctx, req.GetToken())
	if err != nil {
		return nil, h.handleError(ctx, "resolve session", err)
	}

	return &sessioninternalapi.ResolveSessionResponse{
		UserId: userID.String(),
	}, nil
}

func (h handler) RevokeSession(ctx context.Context, req *sessioninternalapi.RevokeSessionRequest) (*sessioninternalapi.RevokeSessionResponse, error) {
	if err := h.svc.RevokeByToken(ctx, req.GetToken()); err != nil {
		return nil, h.handleError(ctx, "revoke session", err)
	}

	return &sessioninternalapi.RevokeSessionResponse{}, nil
}

func (h handler) RevokeOthersByUserID(ctx context.Context, req *sessioninternalapi.RevokeOthersByUserIDRequest) (*sessioninternalapi.RevokeOthersByUserIDResponse, error) {
	userID, err := uuid.Parse(req.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid user ID")
	}

	if err := h.svc.RevokeOthersByUserID(ctx, userID, req.GetToken()); err != nil {
		return nil, h.handleError(ctx, "revoke other sessions", err)
	}

	return &sessioninternalapi.RevokeOthersByUserIDResponse{}, nil
}

func (h handler) handleError(ctx context.Context, operation string, err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidUserID),
		errors.Is(err, domain.ErrInvalidToken):
		return status.Error(codes.InvalidArgument, "invalid argument")

	case errors.Is(err, domain.ErrSessionNotFound),
		errors.Is(err, domain.ErrSessionExpired):
		return status.Error(codes.Unauthenticated, "invalid or expired session")

	default:
		h.logger.ErrorContext(ctx, operation+" failed", logging.Error(err))
		return status.Error(codes.Internal, "internal server error")
	}
}

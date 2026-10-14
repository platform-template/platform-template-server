package internalapi

import (
	"context"
	"errors"
	"log/slog"

	"github.com/distributed-programming-2026/go-sdk/pkg/logging"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityinternalapi "identity/api/server/internalapi"
	"identity/internal/identity/app"
	"identity/internal/identity/domain"
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

func (h handler) CreateUser(
	ctx context.Context,
	req *identityinternalapi.CreateUserRequest,
) (*identityinternalapi.CreateUserResponse, error) {
	userID, err := h.svc.Create(
		ctx,
		req.GetFirstName(),
		req.GetLastName(),
		req.GetMiddleName(),
		req.GetLogin(),
		req.GetEmail(),
	)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrLoginAlreadyExists):
			return nil, status.Error(codes.AlreadyExists, "login already exists")
		case errors.Is(err, domain.ErrInvalidEmail),
			errors.Is(err, domain.ErrInvalidFirstName),
			errors.Is(err, domain.ErrInvalidLastName),
			errors.Is(err, domain.ErrInvalidLogin):
			return nil, status.Error(codes.InvalidArgument, "invalid user data")
		default:
			h.logger.ErrorContext(ctx, "create user failed", logging.Error(err))
			return nil, status.Error(codes.Internal, "create user failed")
		}
	}

	return &identityinternalapi.CreateUserResponse{
		UserId: userID.String(),
	}, nil
}

func (h handler) FindUserByLogin(
	ctx context.Context,
	req *identityinternalapi.FindUserByLoginRequest,
) (*identityinternalapi.FindUserByLoginResponse, error) {
	userID, err := h.svc.FindByLogin(ctx, req.GetLogin())
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrUserNotFound):
			return nil, status.Error(codes.NotFound, "user not found")
		case errors.Is(err, domain.ErrInvalidLogin):
			return nil, status.Error(codes.InvalidArgument, "invalid login")
		default:
			h.logger.ErrorContext(ctx, "find user by login failed", logging.Error(err))
			return nil, status.Error(codes.Internal, "find user by login failed")
		}
	}

	return &identityinternalapi.FindUserByLoginResponse{
		UserId: userID.String(),
	}, nil
}

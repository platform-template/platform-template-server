package internalapi

import (
	"context"
	"errors"
	"log/slog"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/logging"
	"github.com/distributed-programming-2026/lib/transaction/xa"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityinternalapi "identity/api/server/internalapi"
	"identity/internal/identity/app"
	"identity/internal/identity/domain"
)

func NewHandler(
	svc *app.Service,
	registration *app.RegistrationService,
	logger *slog.Logger,
) identityinternalapi.IdentityServiceServer {
	return &handler{
		svc:          svc,
		registration: registration,
		logger:       logger,
	}
}

type handler struct {
	svc          *app.Service
	registration *app.RegistrationService
	logger       *slog.Logger

	identityinternalapi.UnimplementedIdentityServiceServer
}

func (h *handler) CreateUser(
	ctx context.Context,
	req *identityinternalapi.CreateUserRequest,
) (*identityinternalapi.CreateUserResponse, error) {
	userID := uuid.New()

	err := h.svc.CreateWithID(
		ctx,
		userID,
		req.GetFirstName(),
		req.GetLastName(),
		req.MiddleName,
		req.GetLogin(),
		req.GetEmail(),
	)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrLoginAlreadyExists):
			return nil, status.Error(codes.AlreadyExists, "login already exists")
		case isInvalidUserData(err):
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

func (h *handler) FindUserByLogin(
	ctx context.Context,
	req *identityinternalapi.FindUserByLoginRequest,
) (*identityinternalapi.FindUserByLoginResponse, error) {
	userID, err := h.svc.FindByLoginSnapshot(ctx, req.GetLogin())
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

func (h *handler) PrepareRegistration(
	ctx context.Context,
	req *identityinternalapi.PrepareRegistrationRequest,
) (*identityinternalapi.RegistrationTransactionResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	transactionID, err := parseID(req.GetTransactionId(), "transaction_id")
	if err != nil {
		return nil, err
	}

	userID, err := parseID(req.GetUserId(), "user_id")
	if err != nil {
		return nil, err
	}

	err = h.registration.Prepare(
		ctx,
		transactionID,
		userID,
		req.GetFirstName(),
		req.GetLastName(),
		req.MiddleName,
		req.GetLogin(),
		req.GetEmail(),
	)
	if err != nil {
		switch {
		case isInvalidUserData(err):
			return nil, status.Error(codes.InvalidArgument, "invalid user data")
		default:
			return nil, h.registrationError(ctx, "prepare", err)
		}
	}

	return &identityinternalapi.RegistrationTransactionResponse{}, nil
}

func (h *handler) CommitRegistration(
	ctx context.Context,
	req *identityinternalapi.RegistrationTransactionRequest,
) (*identityinternalapi.RegistrationTransactionResponse, error) {
	transactionID, err := parseID(req.GetTransactionId(), "transaction_id")
	if err != nil {
		return nil, err
	}

	if err := h.registration.Commit(ctx, transactionID); err != nil {
		return nil, h.registrationError(ctx, "commit", err)
	}

	return &identityinternalapi.RegistrationTransactionResponse{}, nil
}

func (h *handler) RollbackRegistration(
	ctx context.Context,
	req *identityinternalapi.RegistrationTransactionRequest,
) (*identityinternalapi.RegistrationTransactionResponse, error) {
	transactionID, err := parseID(req.GetTransactionId(), "transaction_id")
	if err != nil {
		return nil, err
	}

	if err := h.registration.Rollback(ctx, transactionID); err != nil {
		return nil, h.registrationError(ctx, "rollback", err)
	}

	return &identityinternalapi.RegistrationTransactionResponse{}, nil
}

func parseID(value string, field string) (uuid.UUID, error) {
	id, err := uuid.Parse(value)
	if err != nil || id == uuid.Nil() {
		return uuid.Nil(), status.Errorf(
			codes.InvalidArgument,
			"%s must be a valid non-nil UUID",
			field,
		)
	}

	return id, nil
}

func isInvalidUserData(err error) bool {
	return errors.Is(err, domain.ErrInvalidUserID) ||
		errors.Is(err, domain.ErrInvalidFirstName) ||
		errors.Is(err, domain.ErrInvalidLastName) ||
		errors.Is(err, domain.ErrInvalidLogin) ||
		errors.Is(err, domain.ErrInvalidEmail)
}

func (h *handler) registrationError(ctx context.Context, operation string, err error) error {
	h.logger.ErrorContext(
		ctx,
		"registration transaction failed",
		slog.String("operation", operation),
		logging.Error(err),
	)

	if errors.Is(err, xa.ErrOutcomeUnknown) {
		return status.Error(
			codes.Unknown,
			"XA transaction outcome unknown; reconciliation required",
		)
	}

	return status.Error(codes.Internal, "registration transaction failed")
}

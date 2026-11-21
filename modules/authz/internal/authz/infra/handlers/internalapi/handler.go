package internalapi

import (
	"context"
	"errors"
	"log/slog"
	"time"
	"uuid"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	internalapi "authz/api/server/internalapi"
	"authz/internal/authz/app"
	"authz/internal/authz/domain"
	sessiongrpc "authz/internal/authz/infra/grpc/session"
)

type sessionResolver interface {
	Resolve(context.Context, string) (uuid.UUID, error)
}
type service interface {
	CheckAccess(context.Context, uuid.UUID, string) (bool, error)
	ListAccess(
		context.Context,
		uuid.UUID,
	) ([]domain.ModuleGrantData, bool, error)
	IsSuperuser(context.Context, uuid.UUID) (bool, error)
}

type handler struct {
	internalapi.UnimplementedAuthzServiceServer
	svc      service
	sessions sessionResolver
	logger   *slog.Logger
}

func NewHandler(
	svc service,
	sessions sessionResolver,
	logger *slog.Logger,
) internalapi.AuthzServiceServer {
	return &handler{svc: svc, sessions: sessions, logger: logger}
}

func (h *handler) CheckAccess(
	ctx context.Context,
	req *internalapi.CheckAccessRequest,
) (*internalapi.CheckAccessResponse, error) {
	actorID, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	targetID, err := parseID(req.GetUserId())
	if err != nil {
		return nil, err
	}
	if err := h.mayRead(ctx, actorID, targetID); err != nil {
		return nil, err
	}
	allowed, err := h.svc.CheckAccess(ctx, targetID, req.GetModuleKey())
	if err != nil {
		return nil, h.handleError(err)
	}
	return &internalapi.CheckAccessResponse{Allowed: allowed}, nil
}

func (h *handler) ListAccess(
	ctx context.Context,
	req *internalapi.ListAccessRequest,
) (*internalapi.ListAccessResponse, error) {
	actorID, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	targetID, err := parseID(req.GetUserId())
	if err != nil {
		return nil, err
	}
	if err := h.mayRead(ctx, actorID, targetID); err != nil {
		return nil, err
	}
	grants, superuser, err := h.svc.ListAccess(ctx, targetID)
	if err != nil {
		return nil, h.handleError(err)
	}
	response := &internalapi.ListAccessResponse{
		IsSuperuser: superuser,
		Grants:      make([]*internalapi.ModuleGrant, len(grants)),
	}
	for i, grant := range grants {
		response.Grants[i] = grantResponse(grant)
	}
	return response, nil
}

func (h *handler) IsSuperuser(
	ctx context.Context,
	_ *internalapi.IsSuperuserRequest,
) (*internalapi.IsSuperuserResponse, error) {
	actorID, err := h.actor(ctx)
	if err != nil {
		return nil, err
	}
	superuser, err := h.svc.IsSuperuser(ctx, actorID)
	if err != nil {
		return nil, h.handleError(err)
	}
	return &internalapi.IsSuperuserResponse{IsSuperuser: superuser}, nil
}

func (h *handler) actor(ctx context.Context) (uuid.UUID, error) {
	values, ok := metadata.FromIncomingContext(ctx)
	if !ok || len(values.Get("session")) != 1 {
		return uuid.Nil(), status.Error(
			codes.Unauthenticated,
			"authentication required",
		)
	}
	userID, err := h.sessions.Resolve(ctx, values.Get("session")[0])
	if err != nil {
		if errors.Is(err, sessiongrpc.ErrInvalidSession) {
			return uuid.Nil(), status.Error(
				codes.Unauthenticated,
				"invalid session",
			)
		}
		return uuid.Nil(), status.Error(
			codes.Unavailable,
			"session unavailable",
		)
	}
	return userID, nil
}

func (h *handler) mayRead(
	ctx context.Context,
	actorID, targetID uuid.UUID,
) error {
	if actorID == targetID {
		return nil
	}
	superuser, err := h.svc.IsSuperuser(ctx, actorID)
	if err != nil {
		return h.handleError(err)
	}
	if !superuser {
		return status.Error(codes.PermissionDenied, "permission denied")
	}
	return nil
}

func (h *handler) handleError(err error) error {
	switch {
	case errors.Is(err, app.ErrPermissionDenied):
		return status.Error(codes.PermissionDenied, "permission denied")
	case errors.Is(err, domain.ErrInvalidUserID),
		errors.Is(err, domain.ErrInvalidModuleKey):
		return status.Error(codes.InvalidArgument, "invalid argument")
	default:
		return status.Error(codes.Internal, "internal server error")
	}
}

func parseID(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil() {
		return uuid.Nil(), status.Error(
			codes.InvalidArgument,
			"invalid user ID",
		)
	}
	return id, nil
}
func grantResponse(grant domain.ModuleGrantData) *internalapi.ModuleGrant {
	response := &internalapi.ModuleGrant{
		UserId:    grant.UserID.String(),
		ModuleKey: grant.ModuleKey.String(),
		GrantedAt: grant.GrantedAt.Format(time.RFC3339Nano),
	}
	if grant.GrantedBy != nil {
		value := grant.GrantedBy.String()
		response.GrantedBy = &value
	}
	return response
}

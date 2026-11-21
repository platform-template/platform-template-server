package public

import (
	"context"
	"errors"
	"uuid"

	publicapi "authz/api/server/publicapi"
	"authz/internal/authz/app"
	"authz/internal/authz/domain"
	sessiongrpc "authz/internal/authz/infra/grpc/session"
)

type sessionResolver interface {
	Resolve(context.Context, string) (uuid.UUID, error)
}
type service interface {
	GrantAccess(context.Context, uuid.UUID, uuid.UUID, string) error
	RevokeAccess(context.Context, uuid.UUID, uuid.UUID, string) error
}
type handler struct {
	svc      service
	sessions sessionResolver
}

func NewHandler(
	svc service,
	sessions sessionResolver,
) publicapi.StrictServerInterface {
	return &handler{svc: svc, sessions: sessions}
}

func (h *handler) GrantAccess(
	ctx context.Context,
	req publicapi.GrantAccessRequestObject,
) (publicapi.GrantAccessResponseObject, error) {
	if req.Body == nil {
		return publicapi.GrantAccess400Response{}, nil
	}
	actorID, err := h.actor(ctx)
	if err != nil {
		return grantAuthError(err), nil
	}
	if err := h.svc.GrantAccess(
		ctx,
		actorID,
		uuid.UUID(req.Body.TargetUserId),
		req.Body.ModuleKey,
	); err != nil {
		return grantError(err), nil
	}
	return publicapi.GrantAccess204Response{}, nil
}

func (h *handler) RevokeAccess(
	ctx context.Context,
	req publicapi.RevokeAccessRequestObject,
) (publicapi.RevokeAccessResponseObject, error) {
	actorID, err := h.actor(ctx)
	if err != nil {
		return revokeAuthError(err), nil
	}
	if err := h.svc.RevokeAccess(
		ctx,
		actorID,
		uuid.UUID(req.UserId),
		req.ModuleKey,
	); err != nil {
		return revokeError(err), nil
	}
	return publicapi.RevokeAccess204Response{}, nil
}

var errAuthentication = errors.New("authentication failed")

func (h *handler) actor(ctx context.Context) (uuid.UUID, error) {
	token, _ := ctx.Value(sessionTokenKey{}).(string)
	if token == "" {
		return uuid.Nil(), errAuthentication
	}
	userID, err := h.sessions.Resolve(ctx, token)
	if err != nil {
		if errors.Is(err, sessiongrpc.ErrInvalidSession) {
			return uuid.Nil(), errAuthentication
		}
		return uuid.Nil(), err
	}
	return userID, nil
}
func grantAuthError(err error) publicapi.GrantAccessResponseObject {
	if errors.Is(err, errAuthentication) {
		return publicapi.GrantAccess401Response{}
	}
	return publicapi.GrantAccess500Response{}
}
func revokeAuthError(err error) publicapi.RevokeAccessResponseObject {
	if errors.Is(err, errAuthentication) {
		return publicapi.RevokeAccess401Response{}
	}
	return publicapi.RevokeAccess500Response{}
}
func grantError(err error) publicapi.GrantAccessResponseObject {
	switch {
	case errors.Is(err, app.ErrPermissionDenied):
		return publicapi.GrantAccess403Response{}
	case errors.Is(err, domain.ErrInvalidUserID),
		errors.Is(err, domain.ErrInvalidModuleKey):
		return publicapi.GrantAccess400Response{}
	default:
		return publicapi.GrantAccess500Response{}
	}
}
func revokeError(err error) publicapi.RevokeAccessResponseObject {
	switch {
	case errors.Is(err, app.ErrPermissionDenied):
		return publicapi.RevokeAccess403Response{}
	case errors.Is(err, domain.ErrInvalidUserID),
		errors.Is(err, domain.ErrInvalidModuleKey):
		return publicapi.RevokeAccess400Response{}
	default:
		return publicapi.RevokeAccess500Response{}
	}
}

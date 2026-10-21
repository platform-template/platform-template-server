package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"uuid"

	publicapi "identity/api/server/publicapi"
	"identity/internal/identity/app"
	"identity/internal/identity/domain"

	googleuuid "github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

const authUserHeader = "X-Auth-User"

type userIDKey struct{}

type handler struct {
	svc *app.Service
}

func NewHandler(svc *app.Service) publicapi.StrictServerInterface {
	return &handler{svc: svc}
}

func (h *handler) GetCurrentIdentity(
	ctx context.Context,
	_ publicapi.GetCurrentIdentityRequestObject,
) (publicapi.GetCurrentIdentityResponseObject, error) {
	userID, ok := userIDFromContext(ctx)
	if !ok {
		return publicapi.GetCurrentIdentity401Response{}, nil
	}

	user, err := h.svc.FindByID(ctx, userID)
	if err != nil {
		if errors.Is(err, domain.ErrUserNotFound) {
			return publicapi.GetCurrentIdentity404Response{}, nil
		}
		return nil, err
	}

	response, err := identityResponse(user)
	if err != nil {
		return nil, err
	}
	return publicapi.GetCurrentIdentity200JSONResponse(response), nil
}

func (h *handler) UpdateCurrentIdentity(
	ctx context.Context,
	req publicapi.UpdateCurrentIdentityRequestObject,
) (publicapi.UpdateCurrentIdentityResponseObject, error) {
	userID, ok := userIDFromContext(ctx)
	if !ok {
		return publicapi.UpdateCurrentIdentity401Response{}, nil
	}
	if req.Body == nil {
		return publicapi.UpdateCurrentIdentity400Response{}, nil
	}

	user, err := h.svc.Update(
		ctx,
		userID,
		req.Body.FirstName,
		req.Body.LastName,
		req.Body.MiddleName,
		string(req.Body.Email),
	)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrUserNotFound):
			return publicapi.UpdateCurrentIdentity404Response{}, nil
		case errors.Is(err, domain.ErrInvalidFirstName),
			errors.Is(err, domain.ErrInvalidLastName),
			errors.Is(err, domain.ErrInvalidEmail):
			return publicapi.UpdateCurrentIdentity400Response{}, nil
		default:
			return nil, err
		}
	}

	response, err := identityResponse(user)
	if err != nil {
		return nil, err
	}
	return publicapi.UpdateCurrentIdentity200JSONResponse(response), nil
}

func withAuthenticatedUser(next publicapi.StrictHandlerFunc, operationID string) publicapi.StrictHandlerFunc {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
		userID, err := uuid.Parse(r.Header.Get(authUserHeader))
		if err != nil || userID == uuid.Nil() {
			switch operationID {
			case "updateCurrentIdentity":
				return publicapi.UpdateCurrentIdentity401Response{}, nil
			case "getCurrentIdentity":
				return publicapi.GetCurrentIdentity401Response{}, nil
			default:
				return nil, fmt.Errorf("unknown protected operation: %s", operationID)
			}
		}

		return next(context.WithValue(ctx, userIDKey{}, userID), w, r, request)
	}
}

func userIDFromContext(ctx context.Context) (uuid.UUID, bool) {
	userID, ok := ctx.Value(userIDKey{}).(uuid.UUID)
	return userID, ok && userID != uuid.Nil()
}

func identityResponse(user *domain.User) (publicapi.Identity, error) {
	data := user.Data()
	userID, err := googleuuid.Parse(data.ID.String())
	if err != nil {
		return publicapi.Identity{}, fmt.Errorf("identity response: parse user ID: %w", err)
	}

	return publicapi.Identity{
		UserId:     userID,
		Login:      data.Login,
		FirstName:  data.FirstName,
		LastName:   data.LastName,
		MiddleName: data.MiddleName,
		Email:      openapi_types.Email(data.Email.String()),
	}, nil
}

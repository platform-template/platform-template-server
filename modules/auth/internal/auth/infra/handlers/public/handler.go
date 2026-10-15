package public

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	publicapi "auth/api/server/publicapi"
	"auth/internal/auth/app"
	"auth/internal/auth/domain"
)

const (
	sessionCookieName = "session"
	sessionMaxAge     = 1 * 24 * 60 * 60
)

type sessionTokenKey struct{}

type handler struct {
	svc *app.Service
}

func NewHandler(svc *app.Service) publicapi.StrictServerInterface {
	return &handler{svc: svc}
}

func (h *handler) Register(
	ctx context.Context,
	req publicapi.RegisterRequestObject,
) (publicapi.RegisterResponseObject, error) {
	if req.Body == nil {
		return authResponse{status: http.StatusBadRequest}, nil
	}

	body := req.Body
	if strings.TrimSpace(body.FirstName) == "" ||
		strings.TrimSpace(body.LastName) == "" ||
		strings.TrimSpace(body.Login) == "" ||
		strings.TrimSpace(string(body.Email)) == "" {
		return authResponse{status: http.StatusBadRequest}, nil
	}

	middleName := ""
	if body.MiddleName != nil {
		middleName = *body.MiddleName
	}

	userID, err := h.svc.Register(ctx, app.RegisterInput{
		FirstName:  body.FirstName,
		LastName:   body.LastName,
		MiddleName: middleName,
		Login:      body.Login,
		Email:      string(body.Email),
		Password:   body.Password,
	})
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidPassword),
			errors.Is(err, app.ErrInvalidUserData):
			return authResponse{status: http.StatusBadRequest}, nil
		case errors.Is(err, app.ErrLoginAlreadyExists),
			errors.Is(err, domain.ErrCredentialAlreadyExists):
			return authResponse{status: http.StatusConflict}, nil
		default:
			return nil, err
		}
	}

	return authResponse{
		status: http.StatusCreated,
		userID: userID.String(),
	}, nil
}

func (h *handler) Login(ctx context.Context, req publicapi.LoginRequestObject) (publicapi.LoginResponseObject, error) {
	if req.Body == nil ||
		strings.TrimSpace(req.Body.Login) == "" ||
		req.Body.Password == "" {
		return authResponse{status: http.StatusBadRequest}, nil
	}

	token, err := h.svc.Login(ctx, req.Body.Login, req.Body.Password)
	if err != nil {
		if errors.Is(err, app.ErrInvalidCredentials) || errors.Is(err, domain.ErrCredentialNotFound) {
			return authResponse{status: http.StatusUnauthorized}, nil
		}
		return nil, err
	}

	return authResponse{
		status: http.StatusNoContent,
		token:  token,
	}, nil
}

func (h *handler) Logout(
	ctx context.Context,
	_ publicapi.LogoutRequestObject,
) (publicapi.LogoutResponseObject, error) {
	token := sessionTokenFromContext(ctx)
	if token != "" {
		if err := h.svc.Logout(ctx, token); err != nil {
			if !errors.Is(err, app.ErrInvalidSession) {
				return nil, err
			}
		}
	}

	return authResponse{status: http.StatusNoContent, clearCookie: true}, nil
}

func (h *handler) ChangePassword(
	ctx context.Context,
	req publicapi.ChangePasswordRequestObject,
) (publicapi.ChangePasswordResponseObject, error) {
	token := sessionTokenFromContext(ctx)
	if token == "" {
		return authResponse{status: http.StatusUnauthorized}, nil
	}
	if req.Body == nil {
		return authResponse{status: http.StatusBadRequest}, nil
	}

	err := h.svc.ChangePassword(
		ctx,
		token,
		req.Body.OldPassword,
		req.Body.NewPassword,
	)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidPassword):
			return authResponse{status: http.StatusBadRequest}, nil
		case errors.Is(err, app.ErrInvalidCredentials),
			errors.Is(err, app.ErrInvalidSession),
			errors.Is(err, domain.ErrCredentialNotFound):
			return authResponse{status: http.StatusUnauthorized}, nil
		case errors.Is(err, domain.ErrPasswordUnchanged):
			return authResponse{status: http.StatusConflict}, nil
		default:
			return nil, err
		}
	}

	return authResponse{status: http.StatusNoContent}, nil
}

func sessionTokenFromContext(ctx context.Context) string {
	token, _ := ctx.Value(sessionTokenKey{}).(string)
	return token
}

type authResponse struct {
	status      int
	userID      string
	token       string
	clearCookie bool
}

func (r authResponse) VisitRegisterResponse(w http.ResponseWriter) error {
	if r.status != http.StatusCreated {
		return writeStatus(w, r.status)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)

	return json.NewEncoder(w).Encode(struct {
		UserID string `json:"userId"`
	}{UserID: r.userID})
}

func (r authResponse) VisitLoginResponse(w http.ResponseWriter) error {
	if r.status == http.StatusNoContent {
		http.SetCookie(w, newSessionCookie(r.token, sessionMaxAge))
	}
	return writeStatus(w, r.status)
}

func (r authResponse) VisitLogoutResponse(w http.ResponseWriter) error {
	if r.clearCookie {
		http.SetCookie(w, newSessionCookie("", -1))
	}
	return writeStatus(w, r.status)
}

func (r authResponse) VisitChangePasswordResponse(w http.ResponseWriter) error {
	return writeStatus(w, r.status)
}

func writeStatus(w http.ResponseWriter, status int) error {
	if status == http.StatusNoContent {
		w.WriteHeader(status)
		return nil
	}

	http.Error(w, http.StatusText(status), status)
	return nil
}

func newSessionCookie(token string, maxAge int) *http.Cookie {
	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   maxAge,
	}

	if maxAge < 0 {
		cookie.Expires = time.Unix(1, 0)
	}

	return cookie
}

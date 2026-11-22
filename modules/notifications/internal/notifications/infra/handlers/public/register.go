package public

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
	"uuid"

	publicapi "notifications/api/server/publicapi"
	"notifications/internal/notifications/app"
	sessiongrpc "notifications/internal/notifications/infra/grpc/session"

	"github.com/distributed-programming-2026/go-sdk/pkg/logging"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/go-chi/chi/v5"

	runtimehttp "github.com/distributed-programming-2026/lib/runtime/http"
)

type sessionResolver interface {
	Resolve(context.Context, string) (uuid.UUID, error)
}

type userIDKey struct{}

func Register(svc *app.Service, logger *slog.Logger, sessions sessionResolver) (runtimehttp.RegisterFunc, error) {
	spec, err := publicapi.GetSpec()
	if err != nil {
		return nil, err
	}
	validatorRouter, err := legacy.NewRouter(spec)
	if err != nil {
		return nil, err
	}
	return func(router chi.Router) error {
		router.Group(func(router chi.Router) {
			router.Use(authenticateNotifications(sessions))
			router.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					route, params, err := validatorRouter.FindRoute(r)
					if err == nil {
						err = openapi3filter.ValidateRequest(r.Context(), &openapi3filter.RequestValidationInput{
							Request:    r,
							PathParams: params,
							Route:      route,
							Options: &openapi3filter.Options{
								AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
							},
						})
					}
					if err != nil {
						http.Error(w, "invalid request", http.StatusBadRequest)
						return
					}
					next.ServeHTTP(w, r)
				})
			})
			strict := publicapi.NewStrictHandlerWithOptions(NewHandler(svc), nil, publicapi.StrictHTTPServerOptions{
				ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
					logger.ErrorContext(r.Context(), "request failed", logging.Error(err))
					http.Error(w, "internal server error", http.StatusInternalServerError)
				},
			})
			publicapi.HandlerFromMux(strict, router)
		})
		return nil
	}, nil
}

func authenticateNotifications(sessions sessionResolver) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			path := r.URL.Path
			base := "/api/public/v1/notifications"
			if path != base && !strings.HasPrefix(path, base+"/") {
				next.ServeHTTP(w, r)
				return
			}
			cookie, err := r.Cookie("session")
			if err != nil || cookie.Value == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			userID, err := sessions.Resolve(ctx, cookie.Value)
			cancel()
			if errors.Is(err, sessiongrpc.ErrInvalidSession) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if err != nil || userID == uuid.Nil() {
				http.Error(w, "authentication unavailable", http.StatusServiceUnavailable)
				return
			}
			if r.Method == http.MethodPatch && r.Header.Get("Origin") != "" {
				origin, err := url.Parse(r.Header.Get("Origin"))
				if err != nil || origin.Host != r.Host {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
			}
			if r.Method == http.MethodPatch && r.ContentLength != 0 {
				http.Error(w, "invalid request", http.StatusBadRequest)
				return
			}
			ctx = context.WithValue(r.Context(), userIDKey{}, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

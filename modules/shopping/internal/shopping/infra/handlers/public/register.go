package public

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	sessionapi "session/api/server/internalapi"
	"strings"
	"time"
	"uuid"

	publicapi "shopping/api/server/publicapi"
	"shopping/internal/shopping/app"

	"github.com/distributed-programming-2026/go-sdk/pkg/logging"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/go-chi/chi/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	runtimehttp "github.com/distributed-programming-2026/lib/runtime/http"
)

func Register(
	svc *app.Service,
	logger *slog.Logger,
	sessions sessionapi.SessionServiceClient,
) (runtimehttp.RegisterFunc, error) {
	spec, err := publicapi.GetSwagger()
	if err != nil {
		return nil, err
	}
	validatorRouter, err := legacy.NewRouter(spec)
	if err != nil {
		return nil, err
	}
	return func(router chi.Router) error {
		router.Group(func(router chi.Router) {
			router.Use(authenticateLists(sessions))
			router.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					route, params, err := validatorRouter.FindRoute(r)
					if err == nil {
						err = openapi3filter.ValidateRequest(r.Context(), &openapi3filter.RequestValidationInput{
							Request:    r,
							PathParams: params,
							Route:      route,
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
					logger.ErrorContext(r.Context(), "echo failed", logging.Error(err))
					http.Error(w, "internal server error", http.StatusInternalServerError)
				},
			})
			publicapi.HandlerFromMux(strict, router)
		})
		return nil
	}, nil
}

type userIDKey struct{}

func authenticateLists(sessions sessionapi.SessionServiceClient) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			const base = "/api/public/v1/shopping-lists"
			if r.URL.Path != base && !strings.HasPrefix(r.URL.Path, base+"/") {
				next.ServeHTTP(w, r)
				return
			}
			cookie, err := r.Cookie("session")
			if err != nil || cookie.Value == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
			defer cancel()
			resolved, err := sessions.ResolveSession(ctx, &sessionapi.ResolveSessionRequest{Token: cookie.Value})
			if status.Code(err) == codes.Unauthenticated || status.Code(err) == codes.InvalidArgument {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if err != nil || resolved == nil {
				http.Error(w, "authentication unavailable", http.StatusServiceUnavailable)
				return
			}
			userID, err := uuid.Parse(resolved.GetUserId())
			if err != nil || userID == uuid.Nil() {
				http.Error(w, "authentication unavailable", http.StatusServiceUnavailable)
				return
			}
			if r.Method != http.MethodGet && r.Header.Get("Origin") != "" {
				origin, err := url.Parse(r.Header.Get("Origin"))
				if err != nil || origin.Host != r.Host || (origin.Scheme != "https" && origin.Scheme != "http") {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIDKey{}, userID)))
		})
	}
}

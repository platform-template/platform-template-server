package public

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	authzapi "authz/api/server/internalapi"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	publicapi "audit/api/server/publicapi"
	"audit/internal/audit/app"

	"github.com/distributed-programming-2026/go-sdk/pkg/logging"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/go-chi/chi/v5"

	runtimehttp "github.com/distributed-programming-2026/lib/runtime/http"
)

func Register(
	svc *app.Service,
	logger *slog.Logger,
	authz authzapi.AuthzServiceClient,
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
			router.Use(authorizeAudit(authz))
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

func authorizeAudit(authz authzapi.AuthzServiceClient) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/public/v1/audit/entries" &&
				!strings.HasPrefix(r.URL.Path, "/api/public/v1/audit/entries/") {
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
			ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("session", cookie.Value))
			response, err := authz.IsSuperuser(ctx, &authzapi.IsSuperuserRequest{})
			if status.Code(err) == codes.Unauthenticated {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if err != nil || response == nil {
				http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
				return
			}
			if !response.GetIsSuperuser() {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

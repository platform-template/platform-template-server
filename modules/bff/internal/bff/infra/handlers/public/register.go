package public

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	publicapi "bff/api/server/publicapi"
	"bff/internal/bff/app"

	"github.com/distributed-programming-2026/go-sdk/pkg/logging"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/go-chi/chi/v5"

	runtimehttp "github.com/distributed-programming-2026/lib/runtime/http"
)

type sessionTokenKey struct{}

func Register(
	svc *app.Service,
	bootstrap *app.BootstrapService,
	logger *slog.Logger,
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
			router.Use(bootstrapAuthentication)
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
			strict := publicapi.NewStrictHandlerWithOptions(
				NewHandler(svc, bootstrap),
				nil,
				publicapi.StrictHTTPServerOptions{
					ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
						logger.ErrorContext(r.Context(), "echo failed", logging.Error(err))
						http.Error(w, "internal server error", http.StatusInternalServerError)
					},
				},
			)
			publicapi.HandlerFromMux(strict, router)
		})
		return nil
	}, nil
}

func bootstrapAuthentication(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/public/v1/bootstrap" && r.URL.Path != "/api/public/v1/navigation" {
			next.ServeHTTP(w, r)
			return
		}
		cookie, err := r.Cookie("session")
		if err != nil || cookie.Value == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionTokenKey{}, cookie.Value)))
	})
}

func sessionToken(ctx context.Context) (string, error) {
	token, ok := ctx.Value(sessionTokenKey{}).(string)
	if !ok || token == "" {
		return "", app.ErrUnauthenticated
	}
	return token, nil
}

func bootstrapError(err error) int {
	if errors.Is(err, app.ErrUnauthenticated) {
		return http.StatusUnauthorized
	}
	return http.StatusServiceUnavailable
}

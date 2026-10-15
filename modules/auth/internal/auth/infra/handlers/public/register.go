package public

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	publicapi "auth/api/server/publicapi"
	"auth/internal/auth/app"

	"github.com/distributed-programming-2026/go-sdk/pkg/logging"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/go-chi/chi/v5"

	runtimehttp "github.com/distributed-programming-2026/lib/runtime/http"
)

func Register(svc *app.Service, logger *slog.Logger) (runtimehttp.RegisterFunc, error) {
	spec, err := publicapi.GetSwagger()
	if err != nil {
		return nil, err
	}

	validatorRouter, err := legacy.NewRouter(spec)
	if err != nil {
		return nil, err
	}

	validationOptions := &openapi3filter.Options{
		AuthenticationFunc: func(_ context.Context, input *openapi3filter.AuthenticationInput) error {
			if input.SecuritySchemeName != "sessionAuth" {
				return errors.New("unsupported authentication scheme")
			}

			cookie, err := input.RequestValidationInput.Request.Cookie(sessionCookieName)
			if err != nil || cookie.Value == "" {
				return errors.New("missing session cookie")
			}

			return nil
		},
	}

	return func(router chi.Router) error {
		router.Group(func(router chi.Router) {
			router.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/api/public/v1/auth/change-password" {
						cookie, err := r.Cookie(sessionCookieName)
						if err != nil || cookie.Value == "" {
							http.Error(w, "unauthorized", http.StatusUnauthorized)
							return
						}
					}

					route, params, err := validatorRouter.FindRoute(r)
					if err == nil {
						err = openapi3filter.ValidateRequest(r.Context(), &openapi3filter.RequestValidationInput{
							Request:    r,
							PathParams: params,
							Route:      route,
							Options:    validationOptions,
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
				NewHandler(svc),
				[]publicapi.StrictMiddlewareFunc{withSessionToken},
				publicapi.StrictHTTPServerOptions{
					ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
						logger.ErrorContext(r.Context(), "auth request failed", logging.Error(err))
						http.Error(w, "internal server error", http.StatusInternalServerError)
					},
				},
			)

			publicapi.HandlerFromMux(strict, router)
		})

		return nil
	}, nil
}

func withSessionToken(next publicapi.StrictHandlerFunc, _ string) publicapi.StrictHandlerFunc {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
		if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
			ctx = context.WithValue(ctx, sessionTokenKey{}, cookie.Value)
		}

		return next(ctx, w, r, request)
	}
}

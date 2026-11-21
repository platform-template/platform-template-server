package public

import (
	"context"
	"net/http"
	"net/url"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/go-chi/chi/v5"

	publicapi "authz/api/server/publicapi"
	"authz/internal/authz/app"

	runtimehttp "github.com/distributed-programming-2026/lib/runtime/http"
)

const sessionCookieName = "session"

type sessionTokenKey struct{}

func Register(
	svc *app.Service,
	sessions sessionResolver,
) (runtimehttp.RegisterFunc, error) {
	spec, err := publicapi.GetSpec()
	if err != nil {
		return nil, err
	}
	validator, err := legacy.NewRouter(spec)
	if err != nil {
		return nil, err
	}
	return func(router chi.Router) error {
		router.Group(func(router chi.Router) {
			router.Use(func(next http.Handler) http.Handler {
				return http.HandlerFunc(
					func(w http.ResponseWriter, r *http.Request) {
						route, params, err := validator.FindRoute(r)
						if err == nil {
							err = openapi3filter.ValidateRequest(
								r.Context(),
								&openapi3filter.RequestValidationInput{
									Request:    r,
									PathParams: params,
									Route:      route,
									Options: &openapi3filter.Options{
										AuthenticationFunc: openapi3filter.NoopAuthenticationFunc,
									},
								},
							)
						}
						if err != nil {
							http.Error(
								w,
								"invalid request",
								http.StatusBadRequest,
							)
							return
						}
						next.ServeHTTP(w, r)
					},
				)
			})
			router.Use(csrf)
			strict := publicapi.NewStrictHandlerWithOptions(
				NewHandler(svc, sessions),
				[]publicapi.StrictMiddlewareFunc{withSessionToken},
				publicapi.StrictHTTPServerOptions{
					ResponseErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, _ error) {
						http.Error(
							w,
							"internal server error",
							http.StatusInternalServerError,
						)
					},
				},
			)
			publicapi.HandlerFromMux(strict, router)
		})
		return nil
	}, nil
}

func withSessionToken(
	next publicapi.StrictHandlerFunc,
	_ string,
) publicapi.StrictHandlerFunc {
	return func(ctx context.Context, w http.ResponseWriter, r *http.Request, request any) (any, error) {
		if cookie, err := r.Cookie(
			sessionCookieName,
		); err == nil &&
			cookie.Value != "" {
			ctx = context.WithValue(ctx, sessionTokenKey{}, cookie.Value)
		}
		return next(ctx, w, r, request)
	}
}
func csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			parsed, err := url.Parse(origin)
			if err != nil || parsed.Host != r.Host {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

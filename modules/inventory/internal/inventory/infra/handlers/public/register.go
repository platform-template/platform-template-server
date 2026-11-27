package public

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
	"uuid"

	authzapi "authz/api/server/internalapi"
	sessionapi "session/api/server/internalapi"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	publicapi "inventory/api/server/publicapi"
	"inventory/internal/inventory/app"

	"github.com/distributed-programming-2026/go-sdk/pkg/logging"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers/legacy"
	"github.com/go-chi/chi/v5"

	runtimehttp "github.com/distributed-programming-2026/lib/runtime/http"
)

func Register(svc *app.Service, logger *slog.Logger, sessions sessionapi.SessionServiceClient,
	authz authzapi.AuthzServiceClient) (runtimehttp.RegisterFunc, error) {
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
			router.Use(authorizeInventory(sessions, authz))
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
					logger.ErrorContext(r.Context(), "echo failed", logging.Error(err))
					http.Error(w, "internal server error", http.StatusInternalServerError)
				},
			})
			publicapi.HandlerFromMux(strict, router)
		})
		return nil
	}, nil
}

func authorizeInventory(
	sessions sessionapi.SessionServiceClient,
	authz authzapi.AuthzServiceClient,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			products := "/api/public/v1/products"
			inventories := "/api/public/v1/inventories"
			isProducts := r.URL.Path == products || strings.HasPrefix(r.URL.Path, products+"/")
			isInventories := r.URL.Path == inventories || strings.HasPrefix(r.URL.Path, inventories+"/")
			if !isProducts && !isInventories {
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
			userID, httpStatus := resolveUser(ctx, sessions, cookie.Value)
			if httpStatus != 0 {
				http.Error(w, http.StatusText(httpStatus), httpStatus)
				return
			}
			if r.Method != http.MethodGet {
				if !validOrigin(r) {
					http.Error(w, "forbidden", http.StatusForbidden)
					return
				}
				if isProducts && !authorizeWrite(ctx, w, cookie.Value, authz) {
					return
				}
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userIDKey{}, userID)))
		})
	}
}

func resolveUser(
	ctx context.Context,
	sessions sessionapi.SessionServiceClient,
	token string,
) (id uuid.UUID, httpStatus int) {
	resolved, err := sessions.ResolveSession(ctx, &sessionapi.ResolveSessionRequest{Token: token})
	if status.Code(err) == codes.Unauthenticated || status.Code(err) == codes.InvalidArgument {
		return uuid.Nil(), http.StatusUnauthorized
	}
	if err != nil || resolved == nil {
		return uuid.Nil(), http.StatusServiceUnavailable
	}
	userID, err := uuid.Parse(resolved.GetUserId())
	if err != nil || userID == uuid.Nil() {
		return uuid.Nil(), http.StatusServiceUnavailable
	}
	return userID, 0
}

type userIDKey struct{}

func validOrigin(r *http.Request) bool {
	if origin := r.Header.Get("Origin"); origin != "" {
		parsed, err := url.Parse(origin)
		return err == nil && parsed.Host == r.Host && (parsed.Scheme == "http" || parsed.Scheme == "https")
	}
	return true
}

func authorizeWrite(ctx context.Context, w http.ResponseWriter,
	token string, authz authzapi.AuthzServiceClient) bool {
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("session", token))
	response, err := authz.IsSuperuser(ctx, &authzapi.IsSuperuserRequest{})
	if status.Code(err) == codes.Unauthenticated {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return false
	}
	if err != nil || response == nil {
		http.Error(w, "authorization unavailable", http.StatusServiceUnavailable)
		return false
	}
	if !response.GetIsSuperuser() {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

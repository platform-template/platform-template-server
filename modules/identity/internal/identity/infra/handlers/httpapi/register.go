package httpapi

import (
	"log/slog"
	"net/http"

	publicapi "identity/api/server/publicapi"
	"identity/internal/identity/app"

	"github.com/distributed-programming-2026/go-sdk/pkg/logging"
	"github.com/go-chi/chi/v5"

	runtimehttp "github.com/distributed-programming-2026/lib/runtime/http"
)

func Register(svc *app.Service, logger *slog.Logger) (runtimehttp.RegisterFunc, error) {
	return func(router chi.Router) error {
		strict := publicapi.NewStrictHandlerWithOptions(
			NewHandler(svc),
			[]publicapi.StrictMiddlewareFunc{withAuthenticatedUser},
			publicapi.StrictHTTPServerOptions{
				ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
					logger.ErrorContext(r.Context(), "identity HTTP request failed", logging.Error(err))
					http.Error(w, "internal server error", http.StatusInternalServerError)
				},
			},
		)

		publicapi.HandlerFromMux(strict, router)
		return nil
	}, nil
}

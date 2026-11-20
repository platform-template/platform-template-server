//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"e2e/internal/clients"
	identityapi "identity/api/server/internalapi"
	sessionapi "session/api/server/internalapi"
)

func TestServicesReady(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	services := []struct{ name, table string }{
		{"auth", "password_credentials"},
		{"identity", "users"},
		{"session", "sessions"},
	}
	for _, service := range services {
		t.Run(service.name, func(t *testing.T) {
			url, err := env.HTTPAddress(ctx, service.name)
			require.NoError(t, err)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url+"/healthz", nil)
			require.NoError(t, err)
			resp, err := http.DefaultClient.Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusNoContent, resp.StatusCode)

			db, err := env.MySQL(ctx, service.name+"-mysql")
			require.NoError(t, err)
			const query = `SELECT COUNT(*) FROM information_schema.tables
WHERE table_schema = DATABASE() AND table_name = ?`
			var count int
			require.NoError(t, db.QueryRowContext(ctx, query, service.table).Scan(&count))
			require.Equal(t, 1, count)
		})
	}

	t.Run("identity-grpc", func(t *testing.T) {
		client, err := clients.Identity(ctx, env)
		require.NoError(t, err)
		_, err = client.FindUserByLogin(ctx, &identityapi.FindUserByLoginRequest{Login: "e2e-absent"})
		require.Equal(t, codes.NotFound, status.Code(err))
	})
	t.Run("session-grpc", func(t *testing.T) {
		client, err := clients.Session(ctx, env)
		require.NoError(t, err)
		_, err = client.ResolveSession(ctx, &sessionapi.ResolveSessionRequest{Token: "e2e-absent"})
		require.Equal(t, codes.Unauthenticated, status.Code(err))
	})
}

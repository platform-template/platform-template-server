//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityapi "identity/api/server/internalapi"
	sessionapi "session/api/server/internalapi"
)

func TestServicesReady(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()

	tables := map[string]string{
		"auth":     "password_credentials",
		"identity": "users",
		"session":  "sessions",
	}
	for _, name := range []string{"auth", "identity", "session"} {
		t.Run(name, func(t *testing.T) {
			req, err := http.NewRequestWithContext(
				ctx,
				http.MethodGet,
				env.HTTPURLs[name]+"/healthz",
				nil,
			)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := env.HTTP.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusNoContent {
				t.Fatalf("health: %s", resp.Status)
			}

			const query = `SELECT COUNT(*) FROM information_schema.tables
WHERE table_schema = DATABASE() AND table_name = ?`
			var count int
			err = env.DB[name].QueryRowContext(ctx, query, tables[name]).
				Scan(&count)
			if err != nil || count != 1 {
				t.Fatalf("migration table %s: count=%d error=%v", tables[name], count, err)
			}
		})
	}

	t.Run("identity-grpc", func(t *testing.T) {
		_, err := env.Identity.FindUserByLogin(ctx, &identityapi.FindUserByLoginRequest{
			Login: "e2e-absent",
		})
		if status.Code(err) != codes.NotFound {
			t.Fatalf("gRPC returned %v, want %v", err, codes.NotFound)
		}
	})

	t.Run("session-grpc", func(t *testing.T) {
		_, err := env.Session.ResolveSession(ctx, &sessionapi.ResolveSessionRequest{
			Token: "e2e-absent",
		})
		if status.Code(err) != codes.Unauthenticated {
			t.Fatalf("gRPC returned %v, want %v", err, codes.Unauthenticated)
		}
	})
}

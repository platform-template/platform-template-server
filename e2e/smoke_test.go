//go:build e2e

package e2e

import (
	"context"
	"database/sql"
	"net/http"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
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
			checkHealth(t, ctx, name)
			checkMigration(t, ctx, name, tables[name])
		})
	}

	for _, name := range []string{"identity", "session"} {
		t.Run(name+"-grpc", func(t *testing.T) {
			checkGRPC(t, ctx, name)
		})
	}
}

func checkHealth(t *testing.T, ctx context.Context, name string) {
	t.Helper()

	httpAddr, err := address(ctx, services[name], "8082/tcp")
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, "http://"+httpAddr+"/healthz", nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("health: %s", resp.Status)
	}
}

func checkMigration(t *testing.T, ctx context.Context, name, table string) {
	t.Helper()

	dsn, err := databases[name].ConnectionString(ctx, "parseTime=true")
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	const query = `
SELECT COUNT(*) FROM information_schema.tables
WHERE table_schema = DATABASE() AND table_name = ?`
	var count int
	err = db.QueryRowContext(ctx, query, table).Scan(&count)
	if err != nil || count != 1 {
		t.Fatalf("migration table %s: count=%d error=%v", table, count, err)
	}
}

func checkGRPC(t *testing.T, ctx context.Context, name string) {
	t.Helper()

	addr, err := address(ctx, services[name], "8081/tcp")
	if err != nil {
		t.Fatal(err)
	}
	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	want := codes.NotFound
	if name == "identity" {
		_, err = identityapi.NewIdentityServiceClient(conn).FindUserByLogin(
			ctx,
			&identityapi.FindUserByLoginRequest{Login: "e2e-absent"},
		)
	} else {
		_, err = sessionapi.NewSessionServiceClient(conn).ResolveSession(
			ctx,
			&sessionapi.ResolveSessionRequest{Token: "e2e-absent"},
		)
		want = codes.Unauthenticated
	}
	if status.Code(err) != want {
		t.Fatalf("gRPC returned %v, want %v", err, want)
	}
}

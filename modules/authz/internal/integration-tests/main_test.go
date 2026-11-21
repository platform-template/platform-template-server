package integration_tests

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	mysqlsdk "github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"

	"authz/internal/authz/infra/mysql/migrations"
)

var testClient mysqlsdk.TransactionalClient

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)

	container, err := tcmysql.Run(ctx, "mysql:8.4", tcmysql.WithDatabase("authz_test"), tcmysql.WithUsername("authz"), tcmysql.WithPassword("integration-password"))
	if err != nil {
		cancel()
		os.Exit(1)
	}
	dsn, err := container.ConnectionString(ctx, "parseTime=true")
	if err != nil {
		_ = container.Terminate(context.Background())
		cancel()
		os.Exit(1)
	}
	config, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		_ = container.Terminate(context.Background())
		cancel()
		os.Exit(1)
	}
	config.ParseTime = true
	connector := mysqlsdk.NewConnector()
	if err := connector.Open(config.FormatDSN(), mysqlsdk.Config{MaxConnections: 16}); err != nil {
		_ = container.Terminate(context.Background())
		cancel()
		os.Exit(1)
	}

	testClient = connector.TransactionalClient()
	runner, err := migrator.New(testClient, slog.Default())
	if err != nil || runner.Migrate(ctx, "authz", migrations.Database(testClient)...) != nil {
		_ = connector.Close()
		_ = container.Terminate(context.Background())
		cancel()
		os.Exit(1)
	}
	cancel()
	exitCode := m.Run()
	_ = connector.Close()
	_ = container.Terminate(context.Background())
	os.Exit(exitCode)
}

func clearTables(t *testing.T) {
	t.Helper()
	_, err := testClient.ExecContext(t.Context(), "test.authz.clear_grants", "DELETE FROM module_grants")
	require.NoError(t, err)
	_, err = testClient.ExecContext(t.Context(), "test.authz.clear_roles", "DELETE FROM system_roles")
	require.NoError(t, err)
}

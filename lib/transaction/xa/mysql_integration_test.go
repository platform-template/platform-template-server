//go:build integration

package xa

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/integrationtests"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var testDB *sql.DB

var testEnv = integrationtests.New(integrationtests.Config{
	Containers: []integrationtests.ContainerSpec{
		{
			Name: "mysql",
			Request: testcontainers.ContainerRequest{
				Image:        "mysql:8.4",
				ExposedPorts: []string{"3306/tcp"},
				Env: map[string]string{
					"MYSQL_ROOT_PASSWORD": "test",
					"MYSQL_DATABASE":      "xa_test",
				},
				WaitingFor: wait.ForLog("port: 3306  MySQL Community Server"),
			},
		},
	},
	Setup: func(ctx context.Context, env *integrationtests.Environment) error {
		endpoint, err := env.HostEndpoint(ctx, "mysql", "3306/tcp")
		if err != nil {
			return err
		}

		dsn := mysqldriver.NewConfig()
		dsn.Net = "tcp"
		dsn.Addr = endpoint
		dsn.User = "root"
		dsn.Passwd = "test"
		dsn.DBName = "xa_test"

		db, err := sql.Open("mysql", dsn.FormatDSN())
		if err != nil {
			return err
		}

		if err := env.AddCleanup("mysql client", func(context.Context) error {
			return db.Close()
		}); err != nil {
			_ = db.Close()
			return err
		}

		db.SetMaxOpenConns(5)
		db.SetMaxIdleConns(0)

		if err := db.PingContext(ctx); err != nil {
			return err
		}

		var detachEnabled int
		err = db.QueryRowContext(ctx, "SELECT @@GLOBAL.xa_detach_on_prepare").Scan(&detachEnabled)
		if err != nil {
			return err
		}
		if detachEnabled != 1 {
			return errors.New("xa_detach_on_prepare must be enabled")
		}

		_, err = db.ExecContext(ctx, `
			CREATE TABLE xa_test_records (
				id CHAR(36) CHARACTER SET ascii PRIMARY KEY,
				value VARCHAR(100) NOT NULL
			) ENGINE=InnoDB
		`)
		if err != nil {
			return err
		}

		testDB = db
		return nil
	},
})

func TestMain(m *testing.M) {
	os.Exit(integrationtests.Run(m, testEnv))
}

func TestMySQLXA(t *testing.T) {
	ctx := testEnv.Bind(t)

	resource, err := NewMySQL(testDB)
	require.NoError(t, err)

	countRecords := func(t *testing.T, ctx context.Context, id uuid.UUID) int {
		t.Helper()

		var count int
		err := testDB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM xa_test_records WHERE id = ?",
			id.String(),
		).Scan(&count)
		require.NoError(t, err)

		return count
	}

	insertRecord := func(id uuid.UUID) func(context.Context, Executor) error {
		return func(ctx context.Context, executor Executor) error {
			_, err := executor.ExecContext(ctx,
				"INSERT INTO xa_test_records (id, value) VALUES (?, ?)",
				id.String(), "test",
			)
			return err
		}
	}

	t.Run("prepare and commit", func(t *testing.T) {
		ctx := testEnv.Bind(t)
		xid := XID{
			TransactionID: uuid.New(),
			Branch:        "auth"
		}

		err := resource.Prepare(ctx, xid, insertRecord(xid.TransactionID))
		require.NoError(t, err)

		assert.Zero(t, countRecords(t, ctx, xid.TransactionID))

		recovered, err := resource.Recover(ctx)
		require.NoError(t, err)
		assert.Contains(t, recovered, xid)

		require.NoError(t, resource.Commit(ctx, xid))
		assert.Equal(t, 1, countRecords(t, ctx, xid.TransactionID))

		recovered, err = resource.Recover(ctx)
		require.NoError(t, err)
		assert.NotContains(t, recovered, xid)
	})

	t.Run("prepare and rollback", func(t *testing.T) {
		ctx := testEnv.Bind(t)
		xid := XID{
			TransactionID: uuid.New(), 
			Branch: "identity"
		}

		err := resource.Prepare(ctx, xid, insertRecord(xid.TransactionID))
		require.NoError(t, err)

		assert.Zero(t, countRecords(t, ctx, xid.TransactionID))

		recovered, err := resource.Recover(ctx)
		require.NoError(t, err)
		assert.Contains(t, recovered, xid)

		require.NoError(t, resource.Rollback(ctx, xid))
		assert.Zero(t, countRecords(t, ctx, xid.TransactionID))

		recovered, err = resource.Recover(ctx)
		require.NoError(t, err)
		assert.NotContains(t, recovered, xid)
	})

	t.Run("operation failure rolls back", func(t *testing.T) {
		ctx := testEnv.Bind(t)
		xid := XID{
			TransactionID: uuid.New(), 
			Branch: "auth"
		}
		operationErr := errors.New("operation failed")

		err := resource.Prepare(ctx, xid, func(ctx context.Context, executor Executor) error {
			if err := insertRecord(xid.TransactionID)(ctx, executor); err != nil {
				return err
			}
			return operationErr
		})
		require.ErrorIs(t, err, operationErr)

		assert.Zero(t, countRecords(t, ctx, xid.TransactionID))

		recovered, err := resource.Recover(ctx)
		require.NoError(t, err)
		assert.NotContains(t, recovered, xid)
	})
}

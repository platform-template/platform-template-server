package integration_tests

import (
	"context"
	"database/sql"
	"testing"
	"time"
	"uuid"

	"github.com/distributed-programming-2026/lib/transaction/xa"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityapi "identity/api/server/internalapi"
)

func countXAUser(t *testing.T, ctx context.Context, userID uuid.UUID) int {
	t.Helper()

	var count int

	err := testEnv.connector.TransactionalClient().GetContext(
		ctx,
		"test.xa.count_user",
		&count,
		"SELECT COUNT(*) FROM users WHERE id = ?",
		userID.String(),
	)
	require.NoError(t, err)

	return count
}

func assertNoPreparedBranch(t *testing.T, ctx context.Context, transactionID uuid.UUID) {
	t.Helper()
	conf := testEnv.conf.MySQL
	dsn := mysqldriver.NewConfig()
	dsn.Net = "tcp"
	dsn.Addr = conf.Address
	dsn.DBName = conf.Database
	dsn.User = "root"
	dsn.Passwd = conf.Password
	db, err := sql.Open("mysql", dsn.FormatDSN())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	resource, err := xa.NewMySQL(db)
	require.NoError(t, err)
	prepared, err := resource.Recover(ctx)
	require.NoError(t, err)
	require.NotContains(t, prepared, xa.XID{TransactionID: transactionID, Branch: "identity"})
}

func TestXARegistration(t *testing.T) {
	client := identityClient(t)

	t.Run("prepare and commit", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		defer cancel()

		transactionID := uuid.New()
		userID := uuid.New()

		_, err := client.PrepareRegistration(ctx, &identityapi.PrepareRegistrationRequest{
			TransactionId: transactionID.String(),
			UserId:        userID.String(),
			FirstName:     "Vitalii",
			LastName:      "Tsal",
			Login:         "xa-commit-" + userID.String(),
			Email:         "tsal@example.com",
		})
		require.NoError(t, err)

		require.Zero(t, countXAUser(t, ctx, userID))

		_, err = client.CommitRegistration(ctx, &identityapi.RegistrationTransactionRequest{
			TransactionId: transactionID.String(),
		})
		require.NoError(t, err)
		require.Equal(t, 1, countXAUser(t, ctx, userID))
		assertNoPreparedBranch(t, ctx, transactionID)

		_, err = client.CommitRegistration(
			ctx,
			&identityapi.RegistrationTransactionRequest{
				TransactionId: transactionID.String(),
			},
		)
		require.NoError(t, err)
		require.Equal(t, 1, countXAUser(t, ctx, userID))
	})

	t.Run("prepare and rollback", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		defer cancel()

		transactionID := uuid.New()
		userID := uuid.New()

		_, err := client.PrepareRegistration(ctx, &identityapi.PrepareRegistrationRequest{
			TransactionId: transactionID.String(),
			UserId:        userID.String(),
			FirstName:     "Vitalii",
			LastName:      "Tsal",
			Login:         "xa-rollback-" + userID.String(),
			Email:         "tsal@example.com",
		})
		require.NoError(t, err)

		require.Zero(t, countXAUser(t, ctx, userID))

		_, err = client.RollbackRegistration(ctx, &identityapi.RegistrationTransactionRequest{
			TransactionId: transactionID.String(),
		})
		require.NoError(t, err)

		require.Zero(t, countXAUser(t, ctx, userID))
		assertNoPreparedBranch(t, ctx, transactionID)
	})

	t.Run("invalid transaction ID", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		_, err := client.CommitRegistration(ctx, &identityapi.RegistrationTransactionRequest{
			TransactionId: "invalid",
		})

		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("commit unknown transaction", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		_, err := client.CommitRegistration(
			ctx,
			&identityapi.RegistrationTransactionRequest{
				TransactionId: uuid.New().String(),
			},
		)

		require.Error(t, err)
	})
}

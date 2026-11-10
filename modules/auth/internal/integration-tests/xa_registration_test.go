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

	"auth/internal/auth/app"
	"auth/internal/auth/domain"
	"auth/internal/auth/infra/password"
)

func newXARegistrationParticipant(
	t *testing.T,
) (*app.RegistrationParticipant, *sql.DB) {
	t.Helper()

	conf := testEnv.conf.MySQL

	dsn := mysqldriver.NewConfig()
	dsn.Net = "tcp"
	dsn.Addr = conf.Address
	dsn.DBName = conf.Database
	dsn.User = conf.User
	dsn.Passwd = conf.Password
	dsn.ParseTime = true

	db, err := sql.Open("mysql", dsn.FormatDSN())
	require.NoError(t, err)

	t.Cleanup(func() {
		require.NoError(t, db.Close())
	})

	require.NoError(t, db.PingContext(t.Context()))

	resource, err := xa.NewMySQL(db)
	require.NoError(t, err)

	return app.NewRegistrationParticipant(
		resource,
		password.NewArgon2Hasher(),
	), db
}

func countCredentials(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	userID uuid.UUID,
) int {
	t.Helper()

	var count int

	err := db.QueryRowContext(
		ctx,
		"SELECT COUNT(*) FROM password_credentials WHERE user_id = ?",
		userID.String(),
	).Scan(&count)

	require.NoError(t, err)

	return count
}

func TestXARegistration(t *testing.T) {
	participant, db := newXARegistrationParticipant(t)

	t.Run("prepare and commit", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		defer cancel()

		transactionID := uuid.New()
		userID := uuid.New()

		err := participant.Prepare(
			ctx,
			transactionID,
			userID,
			"xa-commit-"+userID.String(),
			"secure-password",
		)
		resource, err := xa.NewMySQL(db)
		require.NoError(t, err)

		xid := xa.XID{
			TransactionID: transactionID,
			Branch:        "auth",
		}

		committed, err := resource.Committed(ctx, xid)
		require.NoError(t, err)
		require.False(t, committed)

		require.Zero(t, countCredentials(t, ctx, db, userID))

		err = participant.Commit(ctx, transactionID)
		require.NoError(t, err)

		committed, err = resource.Committed(ctx, xid)
		require.NoError(t, err)
		require.True(t, committed)

		require.Equal(t, 1, countCredentials(t, ctx, db, userID))

		err = participant.Commit(ctx, transactionID)
		require.NoError(t, err)

		require.Equal(t, 1, countCredentials(t, ctx, db, userID))
	})

	t.Run("prepare and rollback", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		defer cancel()

		transactionID := uuid.New()
		userID := uuid.New()

		err := participant.Prepare(
			ctx,
			transactionID,
			userID,
			"xa-rollback-"+userID.String(),
			"secure-password",
		)
		require.NoError(t, err)

		require.Zero(t, countCredentials(t, ctx, db, userID))

		err = participant.Rollback(ctx, transactionID)
		require.NoError(t, err)

		require.Zero(t, countCredentials(t, ctx, db, userID))
	})

	t.Run("invalid user ID", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
		defer cancel()

		err := participant.Prepare(
			ctx,
			uuid.New(),
			uuid.Nil(),
			"invalid-user",
			"secure-password",
		)

		require.ErrorIs(t, err, domain.ErrInvalidUserID)
	})
}

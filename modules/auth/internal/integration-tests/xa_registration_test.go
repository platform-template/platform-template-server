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

func TestXARollbackBeforePrepare(t *testing.T) {
	participant, db := newXARegistrationParticipant(t)

	ctx := t.Context()
	transactionID := uuid.New()
	userID := uuid.New()

	err := participant.Rollback(ctx, transactionID)
	require.NoError(t, err)

	err = participant.Rollback(ctx, transactionID)
	require.NoError(t, err)

	err = participant.Prepare(
		ctx,
		transactionID,
		userID,
		"late-prepare-"+userID.String(),
		"secure-password",
	)
	require.ErrorIs(t, err, xa.ErrBranchAborted)

	require.Zero(t, countCredentials(t, ctx, db, userID))
}

func TestXARollbackAfterCommit(t *testing.T) {
	participant, db := newXARegistrationParticipant(t)

	ctx := t.Context()
	transactionID := uuid.New()
	userID := uuid.New()

	require.NoError(t, participant.Prepare(
		ctx,
		transactionID,
		userID,
		"commit-before-rollback-"+userID.String(),
		"secure-password",
	))

	require.NoError(t, participant.Commit(ctx, transactionID))

	err := participant.Rollback(ctx, transactionID)
	require.ErrorIs(t, err, xa.ErrBranchCommitted)

	require.Equal(t, 1, countCredentials(t, ctx, db, userID))
}

func TestXARollbackWaitsForPrepare(t *testing.T) {
	_, db := newXARegistrationParticipant(t)

	resource, err := xa.NewMySQL(db)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()

	xid := xa.XID{
		TransactionID: uuid.New(),
		Branch:        "auth",
	}

	preparing := make(chan struct{})
	continuePrepare := make(chan struct{})

	prepareDone := make(chan error, 1)
	rollbackDone := make(chan error, 1)

	go func() {
		prepareDone <- resource.Prepare(
			ctx,
			xid,
			func(ctx context.Context, executor xa.Executor) error {
				close(preparing)

				select {
				case <-continuePrepare:
				case <-ctx.Done():
					return ctx.Err()
				}

				return xa.RecordCommitReceipt(ctx, executor, xid)
			},
		)
	}()

	select {
	case <-preparing:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	go func() {
		rollbackDone <- resource.Rollback(ctx, xid)
	}()

	select {
	case err := <-rollbackDone:
		t.Fatalf("rollback finished before prepare: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	close(continuePrepare)

	require.NoError(t, <-prepareDone)
	require.NoError(t, <-rollbackDone)

	committed, err := resource.Committed(ctx, xid)
	require.NoError(t, err)
	require.False(t, committed)

	err = resource.Prepare(
		ctx,
		xid,
		func(context.Context, xa.Executor) error {
			return nil
		},
	)
	require.ErrorIs(t, err, xa.ErrBranchAborted)
}

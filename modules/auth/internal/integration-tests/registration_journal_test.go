package integration_tests

import (
	"database/sql"
	"testing"
	"time"
	"uuid"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"

	"auth/internal/auth/app"
	"auth/internal/auth/infra/mysql/registrationjournal"
)

func newRegistrationJournal(
	t *testing.T,
) (*registrationjournal.RegistrationJournal, *registrationjournal.LeaseStore) {
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

	return registrationjournal.NewRegistrationJournal(db),
		registrationjournal.NewLeaseStore(db)
}

func TestRegistrationJournal(t *testing.T) {
	journal, leases := newRegistrationJournal(t)

	t.Run("commit", func(t *testing.T) {
		ctx := t.Context()

		transactionID := uuid.New()
		userID := uuid.New()

		lease, err := leases.Begin(
			ctx, transactionID, userID, time.Minute,
		)
		require.NoError(t, err)

		tx, err := journal.Get(ctx, transactionID)
		require.NoError(t, err)
		require.Equal(t, userID, tx.UserID)
		require.Equal(t, app.RegistrationPreparing, tx.State)

		require.NoError(t, journal.DecideCommit(ctx, lease))

		tx, err = journal.Get(ctx, transactionID)
		require.NoError(t, err)
		require.Equal(t, app.RegistrationCommitDecided, tx.State)

		err = journal.DecideAbort(ctx, lease)
		require.ErrorIs(t, err, app.ErrRegistrationStateConflict)

		require.NoError(t, journal.MarkCommitted(ctx, lease))

		tx, err = journal.Get(ctx, transactionID)
		require.NoError(t, err)
		require.Equal(t, app.RegistrationCommitted, tx.State)

		err = journal.DecideCommit(ctx, lease)
		require.ErrorIs(t, err, app.ErrRegistrationStateConflict)

		err = journal.MarkCommitted(ctx, lease)
		require.ErrorIs(t, err, app.ErrRegistrationStateConflict)
	})

	t.Run("abort", func(t *testing.T) {
		ctx := t.Context()

		transactionID := uuid.New()

		lease, err := leases.Begin(
			ctx, transactionID, uuid.New(), time.Minute,
		)
		require.NoError(t, err)

		require.NoError(t, journal.DecideAbort(ctx, lease))

		err = journal.DecideCommit(ctx, lease)
		require.ErrorIs(t, err, app.ErrRegistrationStateConflict)

		require.NoError(t, journal.MarkAborted(ctx, lease))

		tx, err := journal.Get(ctx, transactionID)
		require.NoError(t, err)
		require.Equal(t, app.RegistrationAborted, tx.State)

		err = journal.MarkAborted(ctx, lease)
		require.ErrorIs(t, err, app.ErrRegistrationStateConflict)
	})

	t.Run("unfinished transaction survives journal recreation", func(t *testing.T) {
		ctx := t.Context()

		transactionID := uuid.New()
		userID := uuid.New()

		oldLease, err := leases.Begin(
			ctx, transactionID, userID, time.Minute,
		)
		require.NoError(t, err)

		require.NoError(t, journal.DecideCommit(ctx, oldLease))

		recoveredJournal, recoveredLeases := newRegistrationJournal(t)

		pending, err := recoveredJournal.ListPending(ctx)
		require.NoError(t, err)

		var found bool

		for _, tx := range pending {
			if tx.TransactionID == transactionID {
				found = true
				require.Equal(t, userID, tx.UserID)
				require.Equal(t, app.RegistrationCommitDecided, tx.State)
			}
		}

		require.True(
			t,
			found,
			"unfinished transaction must survive journal recreation",
		)

		_, acquired, err := recoveredLeases.Acquire(
			ctx, transactionID, time.Minute,
		)
		require.NoError(t, err)
		require.False(t, acquired)

		released, err := leases.Release(ctx, oldLease)
		require.NoError(t, err)
		require.True(t, released)

		newLease, acquired, err := recoveredLeases.Acquire(
			ctx, transactionID, time.Minute,
		)
		require.NoError(t, err)
		require.True(t, acquired)
		require.Equal(t, oldLease.Epoch+1, newLease.Epoch)

		err = journal.MarkCommitted(ctx, oldLease)
		require.ErrorIs(t, err, app.ErrRegistrationStateConflict)

		require.NoError(t, recoveredJournal.MarkCommitted(ctx, newLease))

		tx, err := journal.Get(ctx, transactionID)
		require.NoError(t, err)
		require.Equal(t, app.RegistrationCommitted, tx.State)
	})

	t.Run("unknown transaction", func(t *testing.T) {
		_, err := journal.Get(t.Context(), uuid.New())
		require.ErrorIs(t, err, app.ErrRegistrationNotFound)
	})
}

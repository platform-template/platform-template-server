package integration_tests

import (
	"database/sql"
	"testing"
	"uuid"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"

	"auth/internal/auth/app"
	registrationjournal "auth/internal/auth/infra/mysql/registrationjournal"
)

func newRegistrationJournal(t *testing.T) *registrationjournal.RegistrationJournal {
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

	return registrationjournal.NewRegistrationJournal(db)
}

func TestRegistrationJournal(t *testing.T) {
	journal := newRegistrationJournal(t)
	ctx := t.Context()

	t.Run("commit", func(t *testing.T) {
		transactionID := uuid.New()
		userID := uuid.New()

		require.NoError(t, journal.Begin(ctx, transactionID, userID))

		tx, err := journal.Get(ctx, transactionID)
		require.NoError(t, err)
		require.Equal(t, userID, tx.UserID)
		require.Equal(t, app.RegistrationPreparing, tx.State)

		require.NoError(t, journal.DecideCommit(ctx, transactionID))

		tx, err = journal.Get(ctx, transactionID)
		require.NoError(t, err)
		require.Equal(t, app.RegistrationCommitDecided, tx.State)

		err = journal.DecideAbort(ctx, transactionID)
		require.ErrorIs(t, err, app.ErrRegistrationStateConflict)

		require.NoError(t, journal.MarkCommitted(ctx, transactionID))

		tx, err = journal.Get(ctx, transactionID)
		require.NoError(t, err)
		require.Equal(t, app.RegistrationCommitted, tx.State)

		require.NoError(t, journal.DecideCommit(ctx, transactionID))
		require.NoError(t, journal.MarkCommitted(ctx, transactionID))
	})

	t.Run("abort", func(t *testing.T) {
		transactionID := uuid.New()
		userID := uuid.New()

		require.NoError(t, journal.Begin(ctx, transactionID, userID))
		require.NoError(t, journal.DecideAbort(ctx, transactionID))

		err := journal.DecideCommit(ctx, transactionID)
		require.ErrorIs(t, err, app.ErrRegistrationStateConflict)

		require.NoError(t, journal.MarkAborted(ctx, transactionID))

		tx, err := journal.Get(ctx, transactionID)
		require.NoError(t, err)
		require.Equal(t, app.RegistrationAborted, tx.State)
	})

	t.Run("unfinished transaction survives journal recreation", func(t *testing.T) {
		transactionID := uuid.New()
		userID := uuid.New()

		require.NoError(t, journal.Begin(ctx, transactionID, userID))
		require.NoError(t, journal.DecideCommit(ctx, transactionID))

		recoveredJournal := newRegistrationJournal(t)

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

		require.True(t, found, "unfinished transaction must be recoverable")

		require.NoError(t, recoveredJournal.MarkCommitted(ctx, transactionID))
	})

	t.Run("unknown transaction", func(t *testing.T) {
		_, err := journal.Get(ctx, uuid.New())
		require.ErrorIs(t, err, app.ErrRegistrationNotFound)
	})
}

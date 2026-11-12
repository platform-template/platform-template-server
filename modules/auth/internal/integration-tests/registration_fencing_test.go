package integration_tests

import (
	"context"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/require"

	"auth/internal/auth/app"
	"auth/internal/auth/infra/mysql/registrationjournal"
)

func TestRegistrationJournalFencingAfterLeaseTakeover(t *testing.T) {
	_, db := newXARegistrationParticipant(t)
	ctx := t.Context()

	leases := registrationjournal.NewLeaseStore(db)
	journal := registrationjournal.NewRegistrationJournal(db)

	oldLease, err := leases.Begin(
		ctx,
		uuid.New(),
		uuid.New(),
		time.Minute,
	)
	require.NoError(t, err)
	require.Equal(t, uint64(1), oldLease.Epoch)

	_, err = db.ExecContext(ctx, `
UPDATE registration_transactions
SET lease_until = UTC_TIMESTAMP(6) - INTERVAL 1 SECOND
WHERE transaction_id = ?
`, oldLease.TransactionID.String())
	require.NoError(t, err)

	newLease, acquired, err := leases.Acquire(
		ctx,
		oldLease.TransactionID,
		time.Minute,
	)
	require.NoError(t, err)
	require.True(t, acquired)
	require.Equal(t, uint64(2), newLease.Epoch)
	require.NotEqual(t, oldLease.OwnerID, newLease.OwnerID)

	start := make(chan struct{})
	oldResult := make(chan error, 1)
	newResult := make(chan error, 1)

	go func() {
		<-start
		oldResult <- journal.DecideCommit(ctx, oldLease)
	}()

	go func() {
		<-start
		newResult <- journal.DecideAbort(ctx, newLease)
	}()

	close(start)

	commitErr := <-oldResult
	abortErr := <-newResult

	require.ErrorIs(
		t,
		commitErr,
		app.ErrRegistrationStateConflict,
	)

	require.NoError(t, abortErr)

	tx, err := journal.Get(ctx, oldLease.TransactionID)
	require.NoError(t, err)
	require.Equal(t, app.RegistrationAbortDecided, tx.State)

	renewed, err := leases.Renew(
		ctx,
		oldLease,
		time.Minute,
	)
	require.NoError(t, err)
	require.False(t, renewed)

	require.NoError(t, journal.MarkAborted(ctx, newLease))

	tx, err = journal.Get(ctx, oldLease.TransactionID)
	require.NoError(t, err)
	require.Equal(t, app.RegistrationAborted, tx.State)
}

func TestRegistrationLeaseConcurrentAcquire(t *testing.T) {
	_, db := newXARegistrationParticipant(t)
	ctx, cancel := context.WithTimeout(
		t.Context(),
		30*time.Second,
	)
	defer cancel()

	leases := registrationjournal.NewLeaseStore(db)

	first, err := leases.Begin(
		ctx,
		uuid.New(),
		uuid.New(),
		time.Minute,
	)
	require.NoError(t, err)

	_, err = db.ExecContext(ctx, `
UPDATE registration_transactions
SET lease_until = UTC_TIMESTAMP(6) - INTERVAL 1 SECOND
WHERE transaction_id = ?
`, first.TransactionID.String())
	require.NoError(t, err)

	const competitors = 10

	type acquireResult struct {
		lease    app.RegistrationLease
		acquired bool
		err      error
	}

	start := make(chan struct{})
	results := make(chan acquireResult, competitors)

	for range competitors {
		go func() {
			<-start

			store := registrationjournal.NewLeaseStore(db)

			lease, acquired, err := store.Acquire(
				ctx,
				first.TransactionID,
				time.Minute,
			)

			results <- acquireResult{
				lease:    lease,
				acquired: acquired,
				err:      err,
			}
		}()
	}

	close(start)

	successful := make([]app.RegistrationLease, 0, 1)

	for range competitors {
		result := <-results
		require.NoError(t, result.err)

		if result.acquired {
			successful = append(successful, result.lease)
		}
	}

	require.Len(t, successful, 1)

	winner := successful[0]

	require.Equal(t, first.TransactionID, winner.TransactionID)
	require.Equal(t, uint64(2), winner.Epoch)
	require.NotEqual(t, first.OwnerID, winner.OwnerID)

	released, err := leases.Release(ctx, first)
	require.NoError(t, err)
	require.False(t, released)

	released, err = leases.Release(ctx, winner)
	require.NoError(t, err)
	require.True(t, released)
}

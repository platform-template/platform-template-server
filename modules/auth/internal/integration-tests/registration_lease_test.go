package integration_tests

import (
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/require"

	"auth/internal/auth/infra/mysql/registrationjournal"
)

func TestRegistrationLeaseFencing(t *testing.T) {
	_, db := newXARegistrationParticipant(t)
	ctx := t.Context()

	transactionID := uuid.New()
	userID := uuid.New()

	_, err := db.ExecContext(ctx, `
INSERT INTO registration_transactions (
    transaction_id, user_id, state
)
VALUES (?, ?, 'PREPARING')
`, transactionID.String(), userID.String())
	require.NoError(t, err)

	store := registrationjournal.NewLeaseStore(db)

	first, acquired, err := store.Acquire(
		ctx, transactionID, 30*time.Second,
	)
	require.NoError(t, err)
	require.True(t, acquired)
	require.Equal(t, uint64(1), first.Epoch)

	_, acquired, err = store.Acquire(
		ctx, transactionID, 30*time.Second,
	)
	require.NoError(t, err)
	require.False(t, acquired)

	_, err = db.ExecContext(ctx, `
UPDATE registration_transactions
SET lease_until = UTC_TIMESTAMP(6) - INTERVAL 1 SECOND
WHERE transaction_id = ?
`, transactionID.String())
	require.NoError(t, err)

	second, acquired, err := store.Acquire(
		ctx, transactionID, 30*time.Second,
	)
	require.NoError(t, err)
	require.True(t, acquired)
	require.Equal(t, uint64(2), second.Epoch)
	require.NotEqual(t, first.OwnerID, second.OwnerID)

	renewed, err := store.Renew(
		ctx, first, 30*time.Second,
	)
	require.NoError(t, err)
	require.False(t, renewed)

	released, err := store.Release(ctx, first)
	require.NoError(t, err)
	require.False(t, released)

	renewed, err = store.Renew(
		ctx, second, 30*time.Second,
	)
	require.NoError(t, err)
	require.True(t, renewed)

	released, err = store.Release(ctx, second)
	require.NoError(t, err)
	require.True(t, released)

	third, acquired, err := store.Acquire(
		ctx, transactionID, 30*time.Second,
	)
	require.NoError(t, err)
	require.True(t, acquired)
	require.Equal(t, uint64(3), third.Epoch)
}

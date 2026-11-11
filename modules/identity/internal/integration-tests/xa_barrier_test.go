package integration_tests

import (
	"context"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/require"

	identityapi "identity/api/server/internalapi"
)

func newXABarrierRequest(
	transactionID, userID uuid.UUID,
) *identityapi.PrepareRegistrationRequest {
	return &identityapi.PrepareRegistrationRequest{
		TransactionId: transactionID.String(),
		UserId:        userID.String(),
		FirstName:     "Alex",
		LastName:      "Smith",
		Login:         "xa-" + userID.String(),
		Email:         "xa-" + userID.String() + "@example.com",
	}
}

func xaBarrierCount(
	t *testing.T,
	ctx context.Context,
	query string,
	args ...any,
) int {
	t.Helper()

	var count int

	err := testEnv.connector.TransactionalClient().GetContext(
		ctx,
		"test.identity.xa_barrier.count",
		&count,
		query,
		args...,
	)
	require.NoError(t, err)

	return count
}

func TestIdentityXARollbackBeforePrepare(t *testing.T) {
	client := identityClient(t)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	transactionID := uuid.New()
	userID := uuid.New()

	txRequest := &identityapi.RegistrationTransactionRequest{
		TransactionId: transactionID.String(),
	}

	_, err := client.RollbackRegistration(ctx, txRequest)
	require.NoError(t, err)

	barriers := xaBarrierCount(t, ctx, `
SELECT COUNT(*)
FROM xa_abort_barriers
WHERE transaction_id = ? AND branch = 'identity'
`, transactionID.String())
	require.Equal(t, 1, barriers)

	_, err = client.PrepareRegistration(
		ctx,
		newXABarrierRequest(transactionID, userID),
	)
	require.Error(t, err)

	users := xaBarrierCount(t, ctx, `
SELECT COUNT(*) FROM users WHERE id = ?
`, userID.String())
	require.Zero(t, users)

	_, err = client.CommitRegistration(ctx, txRequest)
	require.Error(t, err)
}

func TestIdentityXARepeatedRollback(t *testing.T) {
	client := identityClient(t)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	transactionID := uuid.New()
	userID := uuid.New()

	txRequest := &identityapi.RegistrationTransactionRequest{
		TransactionId: transactionID.String(),
	}

	_, err := client.PrepareRegistration(
		ctx,
		newXABarrierRequest(transactionID, userID),
	)
	require.NoError(t, err)

	users := xaBarrierCount(t, ctx, `
SELECT COUNT(*) FROM users WHERE id = ?
`, userID.String())
	require.Zero(t, users)

	_, err = client.RollbackRegistration(ctx, txRequest)
	require.NoError(t, err)

	_, err = client.RollbackRegistration(ctx, txRequest)
	require.NoError(t, err)

	users = xaBarrierCount(t, ctx, `
SELECT COUNT(*) FROM users WHERE id = ?
`, userID.String())
	require.Zero(t, users)

	receipts := xaBarrierCount(t, ctx, `
SELECT COUNT(*)
FROM xa_branch_receipts
WHERE transaction_id = ? AND branch = 'identity'
`, transactionID.String())
	require.Zero(t, receipts)

	barriers := xaBarrierCount(t, ctx, `
SELECT COUNT(*)
FROM xa_abort_barriers
WHERE transaction_id = ? AND branch = 'identity'
`, transactionID.String())
	require.Equal(t, 1, barriers)

	_, err = client.PrepareRegistration(
		ctx,
		newXABarrierRequest(transactionID, userID),
	)
	require.Error(t, err)
}

func TestIdentityXARollbackAfterCommit(t *testing.T) {
	client := identityClient(t)

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	transactionID := uuid.New()
	userID := uuid.New()

	txRequest := &identityapi.RegistrationTransactionRequest{
		TransactionId: transactionID.String(),
	}

	_, err := client.PrepareRegistration(
		ctx,
		newXABarrierRequest(transactionID, userID),
	)
	require.NoError(t, err)

	_, err = client.CommitRegistration(ctx, txRequest)
	require.NoError(t, err)

	_, err = client.CommitRegistration(ctx, txRequest)
	require.NoError(t, err)

	users := xaBarrierCount(t, ctx, `
SELECT COUNT(*) FROM users WHERE id = ?
`, userID.String())
	require.Equal(t, 1, users)

	receipts := xaBarrierCount(t, ctx, `
SELECT COUNT(*)
FROM xa_branch_receipts
WHERE transaction_id = ? AND branch = 'identity'
`, transactionID.String())
	require.Equal(t, 1, receipts)

	_, err = client.RollbackRegistration(ctx, txRequest)
	require.Error(t, err)

	users = xaBarrierCount(t, ctx, `
SELECT COUNT(*) FROM users WHERE id = ?
`, userID.String())
	require.Equal(t, 1, users)

	barriers := xaBarrierCount(t, ctx, `
SELECT COUNT(*)
FROM xa_abort_barriers
WHERE transaction_id = ? AND branch = 'identity'
`, transactionID.String())
	require.Zero(t, barriers)
}

package integration_tests

import (
	"context"
	"testing"
	"time"
	"uuid"

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
	})

	t.Run("invalid transaction ID", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()

		_, err := client.CommitRegistration(ctx, &identityapi.RegistrationTransactionRequest{
			TransactionId: "invalid",
		})

		require.Equal(t, codes.InvalidArgument, status.Code(err))
	})
}

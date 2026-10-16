package integration_tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"uuid"

	sessionapi "session/api/server/internalapi"
	"session/internal/session/domain"
)

func sessionClient(t *testing.T) sessionapi.SessionServiceClient {
	t.Helper()
	connection, err := grpc.NewClient(testEnv.conf.GRPC.Address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, connection.Close()) })
	return sessionapi.NewSessionServiceClient(connection)
}

func resolveSession(t *testing.T, client sessionapi.SessionServiceClient, token string) string {
	t.Helper()
	response, err := client.ResolveSession(t.Context(), &sessionapi.ResolveSessionRequest{Token: token})
	require.NoError(t, err)
	return response.GetUserId()
}

func TestSessionGRPCCreateResolveAndStoreHash(t *testing.T) {
	client := sessionClient(t)
	userID := uuid.New().String()
	response, err := client.CreateSession(t.Context(), &sessionapi.CreateSessionRequest{UserId: userID})
	require.NoError(t, err)
	require.NotEmpty(t, response.GetToken())
	require.Equal(t, userID, resolveSession(t, client, response.GetToken()))

	var storedHash string
	err = testEnv.connector.TransactionalClient().GetContext(t.Context(), "test.session.hash", &storedHash, `
SELECT token_hash FROM sessions WHERE user_id = ?`, userID)
	require.NoError(t, err)
	require.Equal(t, string(domain.HashSessionToken(domain.SessionToken(response.GetToken()))), storedHash)
	require.NotEqual(t, response.GetToken(), storedHash)
}

func TestSessionGRPCRejectsUnknownAndExpiredTokens(t *testing.T) {
	client := sessionClient(t)
	_, err := client.ResolveSession(t.Context(), &sessionapi.ResolveSessionRequest{Token: ""})
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	_, err = client.ResolveSession(t.Context(), &sessionapi.ResolveSessionRequest{Token: "unknown"})
	require.Equal(t, codes.Unauthenticated, status.Code(err))

	userID := uuid.New().String()
	created, err := client.CreateSession(t.Context(), &sessionapi.CreateSessionRequest{UserId: userID})
	require.NoError(t, err)
	_, err = testEnv.connector.TransactionalClient().ExecContext(t.Context(), "test.session.expire", `
UPDATE sessions SET expires_at = ? WHERE token_hash = ?`, time.Now().UTC().Add(-time.Second), domain.HashSessionToken(domain.SessionToken(created.GetToken())))
	require.NoError(t, err)
	_, err = client.ResolveSession(t.Context(), &sessionapi.ResolveSessionRequest{Token: created.GetToken()})
	require.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestSessionGRPCRevocation(t *testing.T) {
	client := sessionClient(t)
	userID := uuid.New().String()
	otherUserID := uuid.New().String()
	current, err := client.CreateSession(t.Context(), &sessionapi.CreateSessionRequest{UserId: userID})
	require.NoError(t, err)
	other, err := client.CreateSession(t.Context(), &sessionapi.CreateSessionRequest{UserId: userID})
	require.NoError(t, err)
	foreign, err := client.CreateSession(t.Context(), &sessionapi.CreateSessionRequest{UserId: otherUserID})
	require.NoError(t, err)

	_, err = client.RevokeOthersByUserID(t.Context(), &sessionapi.RevokeOthersByUserIDRequest{UserId: userID, Token: current.GetToken()})
	require.NoError(t, err)
	require.Equal(t, userID, resolveSession(t, client, current.GetToken()))
	_, err = client.ResolveSession(t.Context(), &sessionapi.ResolveSessionRequest{Token: other.GetToken()})
	require.Equal(t, codes.Unauthenticated, status.Code(err))
	require.Equal(t, otherUserID, resolveSession(t, client, foreign.GetToken()))

	_, err = client.RevokeSession(t.Context(), &sessionapi.RevokeSessionRequest{Token: current.GetToken()})
	require.NoError(t, err)
	_, err = client.ResolveSession(t.Context(), &sessionapi.ResolveSessionRequest{Token: current.GetToken()})
	require.Equal(t, codes.Unauthenticated, status.Code(err))
}

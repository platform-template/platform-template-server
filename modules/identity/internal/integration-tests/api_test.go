package integration_tests

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	identityapi "identity/api/server/internalapi"
)

const testUserLogin = "ada-lovelace"

func identityClient(t *testing.T) identityapi.IdentityServiceClient {
	t.Helper()
	connection, err := grpc.NewClient(testEnv.conf.GRPC.Address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, connection.Close()) })
	return identityapi.NewIdentityServiceClient(connection)
}

func TestIdentityGRPCUserLifecycle(t *testing.T) {
	client := identityClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	created, err := client.CreateUser(ctx, &identityapi.CreateUserRequest{
		FirstName:  "Ada",
		LastName:   "Lovelace",
		MiddleName: "Byron",
		Login:      testUserLogin,
		Email:      "ada@example.com",
	})
	require.NoError(t, err)
	require.NotEmpty(t, created.GetUserId())

	var stored struct {
		FirstName  string `db:"first_name"`
		LastName   string `db:"last_name"`
		MiddleName string `db:"middle_name"`
		Login      string `db:"login"`
		Email      string `db:"email"`
	}
	err = testEnv.connector.TransactionalClient().GetContext(t.Context(), "test.identity.user", &stored, `
SELECT first_name, last_name, middle_name, login, email
FROM users
WHERE id = ?`, created.GetUserId())
	require.NoError(t, err)
	require.Equal(t, "Ada", stored.FirstName)
	require.Equal(t, "Lovelace", stored.LastName)
	require.Equal(t, "Byron", stored.MiddleName)
	require.Equal(t, testUserLogin, stored.Login)
	require.Equal(t, "ada@example.com", stored.Email)

	found, err := client.FindUserByLogin(ctx, &identityapi.FindUserByLoginRequest{Login: testUserLogin})
	require.NoError(t, err)
	require.Equal(t, created.GetUserId(), found.GetUserId())

	_, err = client.CreateUser(ctx, &identityapi.CreateUserRequest{
		FirstName: "Another",
		LastName:  "User",
		Login:     testUserLogin,
		Email:     "another@example.com",
	})
	require.Equal(t, codes.AlreadyExists, status.Code(err))
}

func TestIdentityGRPCRejectsMissingAndInvalidUsers(t *testing.T) {
	client := identityClient(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	_, err := client.FindUserByLogin(ctx, &identityapi.FindUserByLoginRequest{Login: "absent"})
	require.Equal(t, codes.NotFound, status.Code(err))

	_, err = client.CreateUser(ctx, &identityapi.CreateUserRequest{
		FirstName: "Ada",
		LastName:  "Lovelace",
		Login:     "ada",
		Email:     "not-an-email",
	})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"testing"

	identityhttp "identity/api/server/publicapi"

	"github.com/stretchr/testify/require"
)

func profile(t *testing.T, ctx context.Context, cookie *http.Cookie) *identityhttp.GetCurrentIdentityResponse {
	t.Helper()
	address, err := env.GatewayAddress(ctx)
	require.NoError(t, err)
	client, err := identityhttp.NewClientWithResponses(address)
	require.NoError(t, err)
	response, err := client.GetCurrentIdentityWithResponse(ctx, func(_ context.Context, request *http.Request) error {
		request.AddCookie(cookie)
		return nil
	})
	require.NoError(t, err)
	return response
}

func TestRegisterUser(t *testing.T) {
	ctx, client, user := scenario(t)
	response, err := client.RegisterWithResponse(ctx, user)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, response.StatusCode())
	require.NotNil(t, response.JSON201)
	cookie := loggedIn(t, ctx, client, user.Login, user.Password)
	result := profile(t, ctx, cookie)
	require.Equal(t, http.StatusOK, result.StatusCode())
	require.NotNil(t, result.JSON200)
	require.Equal(t, response.JSON201.UserId.String(), result.JSON200.UserId.String())
	require.Equal(t, user.Login, result.JSON200.Login)
	require.Equal(t, user.FirstName, result.JSON200.FirstName)
	require.Equal(t, user.LastName, result.JSON200.LastName)
	require.Equal(t, user.MiddleName, result.JSON200.MiddleName)
	require.Equal(t, user.Email, result.JSON200.Email)
}

func TestRegisterDuplicateLogin(t *testing.T) {
	ctx, client, user := scenario(t)
	id := registeredUser(t, ctx, client, user)
	originalCookie := loggedIn(t, ctx, client, user.Login, user.Password)
	originalProfile := profile(t, ctx, originalCookie)
	require.Equal(t, http.StatusOK, originalProfile.StatusCode())
	require.NotNil(t, originalProfile.JSON200)
	duplicate := user
	duplicate.Email = "duplicate-" + user.Email
	duplicate.Password = "other-password-123"
	duplicate.FirstName = "Other"
	response, err := client.RegisterWithResponse(ctx, duplicate)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, response.StatusCode())
	cookie := loggedIn(t, ctx, client, user.Login, user.Password)
	wrong, err := client.LoginWithResponse(ctx, authLogin(user.Login, duplicate.Password))
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, wrong.StatusCode())
	result := profile(t, ctx, cookie)
	require.Equal(t, http.StatusOK, result.StatusCode())
	require.NotNil(t, result.JSON200)
	require.Equal(t, originalProfile.JSON200, result.JSON200)
	require.Equal(t, id, result.JSON200.UserId.String())
	require.Equal(t, user.FirstName, result.JSON200.FirstName)
	require.Equal(t, user.Email, result.JSON200.Email)
}

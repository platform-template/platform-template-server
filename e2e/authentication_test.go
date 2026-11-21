//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	authapi "auth/api/server/publicapi"

	"github.com/stretchr/testify/require"
)

func TestLoginCreatesSession(t *testing.T) {
	ctx, client, user := scenario(t)
	id := registeredUser(t, ctx, client, user)

	response, err := client.LoginWithResponse(ctx, authapi.LoginRequest{Login: user.Login, Password: user.Password})
	require.NoError(t, err)
	cookie := loginCookie(t, response)

	result := profile(t, ctx, cookie)
	require.Equal(t, http.StatusOK, result.StatusCode())
	require.Equal(t, id, result.JSON200.UserId.String())
}

func TestLoginRejectsInvalidCredentials(t *testing.T) {
	ctx, client, user := scenario(t)
	registeredUser(t, ctx, client, user)

	for _, tc := range []struct{ name, login, password string }{
		{"unknown login", user.Login + "-absent", user.Password},
		{"wrong password", user.Login, "wrong-password"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response, err := client.LoginWithResponse(ctx, authapi.LoginRequest{Login: tc.login, Password: tc.password})
			require.NoError(t, err)
			require.Equal(t, http.StatusUnauthorized, response.StatusCode())
			require.Zero(t, len(response.HTTPResponse.Cookies()))
		})
	}
	cookie := loggedIn(t, ctx, client, user.Login, user.Password)
	require.Equal(t, http.StatusOK, profile(t, ctx, cookie).StatusCode())
}

func TestLogoutRevokesSession(t *testing.T) {
	ctx, client, user := scenario(t)
	registeredUser(t, ctx, client, user)
	cookie := loggedIn(t, ctx, client, user.Login, user.Password)
	require.Equal(t, http.StatusOK, profile(t, ctx, cookie).StatusCode())

	response, err := client.LogoutWithResponse(ctx, withCookie(cookie))
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, response.StatusCode())
	require.Equal(t, http.StatusUnauthorized, profile(t, ctx, cookie).StatusCode())
}

func TestChangePassword(t *testing.T) {
	ctx, client, user := scenario(t)
	id := registeredUser(t, ctx, client, user)
	current := loggedIn(t, ctx, client, user.Login, user.Password)
	other := loggedIn(t, ctx, client, user.Login, user.Password)
	const newPassword = "test-password-2"

	response, err := client.ChangePasswordWithResponse(
		ctx,
		authapi.ChangePasswordRequest{OldPassword: user.Password, NewPassword: newPassword},
		withCookie(current),
	)
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, response.StatusCode())

	currentProfile := profile(t, ctx, current)
	require.Equal(t, http.StatusOK, currentProfile.StatusCode())
	require.Equal(t, id, currentProfile.JSON200.UserId.String())
	require.Equal(t, http.StatusUnauthorized, profile(t, ctx, other).StatusCode())
	oldLogin, err := client.LoginWithResponse(ctx, authapi.LoginRequest{Login: user.Login, Password: user.Password})
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, oldLogin.StatusCode())
	newLogin, err := client.LoginWithResponse(ctx, authapi.LoginRequest{Login: user.Login, Password: newPassword})
	require.NoError(t, err)
	newCookie := loginCookie(t, newLogin)
	newProfile := profile(t, ctx, newCookie)
	require.Equal(t, http.StatusOK, newProfile.StatusCode())
	require.Equal(t, id, newProfile.JSON200.UserId.String())
}

//go:build e2e

package e2e

import (
	"net/http"
	"testing"

	authapi "auth/api/server/publicapi"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestLoginCreatesSession(t *testing.T) {
	ctx, client, user := scenario(t)
	id := registeredUser(t, ctx, client, user)

	response, err := client.LoginWithResponse(ctx, authapi.LoginRequest{Login: user.Login, Password: user.Password})
	require.NoError(t, err)
	cookie := loginCookie(t, response)

	session, err := resolveSession(t, ctx, cookie)
	require.NoError(t, err)
	require.Equal(t, id, session.GetUserId())
}

func TestLoginRejectsInvalidCredentials(t *testing.T) {
	ctx, client, user := scenario(t)
	registeredUser(t, ctx, client, user)
	db, err := env.MySQL(ctx, "session-mysql")
	require.NoError(t, err)
	var before int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sessions").Scan(&before))

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
	var after int
	require.NoError(t, db.QueryRowContext(ctx, "SELECT COUNT(*) FROM sessions").Scan(&after))
	require.Equal(t, before, after)
}

func TestLogoutRevokesSession(t *testing.T) {
	ctx, client, user := scenario(t)
	id := registeredUser(t, ctx, client, user)
	cookie := loggedIn(t, ctx, client, user.Login, user.Password)
	session, err := resolveSession(t, ctx, cookie)
	require.NoError(t, err)
	require.Equal(t, id, session.GetUserId())

	response, err := client.LogoutWithResponse(ctx, withCookie(cookie))
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, response.StatusCode())
	_, err = resolveSession(t, ctx, cookie)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
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

	session, err := resolveSession(t, ctx, current)
	require.NoError(t, err)
	require.Equal(t, id, session.GetUserId())
	_, err = resolveSession(t, ctx, other)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
	oldLogin, err := client.LoginWithResponse(ctx, authapi.LoginRequest{Login: user.Login, Password: user.Password})
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, oldLogin.StatusCode())
	newLogin, err := client.LoginWithResponse(ctx, authapi.LoginRequest{Login: user.Login, Password: newPassword})
	require.NoError(t, err)
	newCookie := loginCookie(t, newLogin)
	session, err = resolveSession(t, ctx, newCookie)
	require.NoError(t, err)
	require.Equal(t, id, session.GetUserId())
}

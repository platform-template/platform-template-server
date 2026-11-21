//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"testing"
	"time"

	authapi "auth/api/server/publicapi"
	"e2e/internal/clients"
	"e2e/internal/fixtures"

	"github.com/stretchr/testify/require"
)

func scenario(t *testing.T) (context.Context, *authapi.ClientWithResponses, authapi.RegisterRequest) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
	t.Cleanup(cancel)
	client, err := clients.Auth(ctx, env)
	require.NoError(t, err)
	user, err := fixtures.User()
	require.NoError(t, err)
	return ctx, client, user
}

func registeredUser(
	t *testing.T,
	ctx context.Context,
	client *authapi.ClientWithResponses,
	user authapi.RegisterRequest,
) string {
	t.Helper()
	response, err := client.RegisterWithResponse(ctx, user)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, response.StatusCode())
	require.NotNil(t, response.JSON201)
	id := response.JSON201.UserId.String()
	require.NotEqual(t, "00000000-0000-0000-0000-000000000000", id)
	return id
}

func loggedIn(
	t *testing.T,
	ctx context.Context,
	client *authapi.ClientWithResponses,
	login, password string,
) *http.Cookie {
	t.Helper()
	response, err := client.LoginWithResponse(ctx, authapi.LoginRequest{Login: login, Password: password})
	require.NoError(t, err)
	return loginCookie(t, response)
}

func loginCookie(t *testing.T, response *authapi.LoginResponse) *http.Cookie {
	t.Helper()
	require.Equal(t, http.StatusNoContent, response.StatusCode())
	cookies := response.HTTPResponse.Cookies()
	require.Equal(t, 1, len(cookies))
	require.Equal(t, "session", cookies[0].Name)
	require.True(t, cookies[0].HttpOnly)
	require.NotEmpty(t, cookies[0].Value)
	return cookies[0]
}

func withCookie(cookie *http.Cookie) authapi.RequestEditorFn {
	return func(_ context.Context, req *http.Request) error { req.AddCookie(cookie); return nil }
}

func authLogin(login, password string) authapi.LoginRequest {
	return authapi.LoginRequest{Login: login, Password: password}
}

package integration_tests

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const authAPIPath = "/api/public/v1/auth"

type registerResponse struct {
	UserID string `json:"userId"`
}

func authRequest(t *testing.T, path string, body any, cookie *http.Cookie) *http.Response {
	t.Helper()
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		require.NoError(t, err)
	}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://"+testEnv.conf.HTTP.Address+path, bytes.NewReader(payload))
	require.NoError(t, err)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		request.AddCookie(cookie)
	}
	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, response.Body.Close()) })
	return response
}

func registerUser(t *testing.T, login, password string) string {
	t.Helper()
	response := authRequest(t, authAPIPath+"/register", map[string]string{
		"firstName": "Ada",
		"lastName":  "Lovelace",
		"login":     login,
		"email":     login + "@example.com",
		"password":  password,
	}, nil)
	require.Equal(t, http.StatusCreated, response.StatusCode)
	var result registerResponse
	require.NoError(t, json.NewDecoder(response.Body).Decode(&result))
	require.NotEmpty(t, result.UserID)
	return result.UserID
}

func login(t *testing.T, login, password string) *http.Cookie {
	t.Helper()
	response := authRequest(t, authAPIPath+"/login", map[string]string{
		"login":    login,
		"password": password,
	}, nil)
	require.Equal(t, http.StatusNoContent, response.StatusCode)
	cookies := response.Cookies()
	require.Len(t, cookies, 1)
	require.Equal(t, "session", cookies[0].Name)
	require.True(t, cookies[0].HttpOnly)
	return cookies[0]
}

func TestAuthHTTPRegisterAndLogin(t *testing.T) {
	userID := registerUser(t, "auth-register", "password-1")
	var passwordHash string
	err := testEnv.connector.TransactionalClient().GetContext(t.Context(), "test.auth.credentials", &passwordHash, `
SELECT password_hash FROM password_credentials WHERE user_id = ?`, userID)
	require.NoError(t, err)
	require.NotEmpty(t, passwordHash)

	response := authRequest(t, authAPIPath+"/register", map[string]string{
		"firstName": "Ada",
		"lastName":  "Lovelace",
		"login":     "auth-register",
		"email":     "second@example.com",
		"password":  "password-1",
	}, nil)
	require.Equal(t, http.StatusConflict, response.StatusCode)

	response = authRequest(t, authAPIPath+"/register", map[string]string{
		"firstName": "Ada",
		"lastName":  "Lovelace",
		"login":     "invalid-email",
		"email":     "not-an-email",
		"password":  "password-1",
	}, nil)
	require.Equal(t, http.StatusBadRequest, response.StatusCode)

	before := testEnv.remotes.session.sessionCount()
	response = authRequest(t, authAPIPath+"/login", map[string]string{
		"login":    "auth-register",
		"password": "wrong-password",
	}, nil)
	require.Equal(t, http.StatusUnauthorized, response.StatusCode)
	require.Equal(t, before, testEnv.remotes.session.sessionCount())

	response = authRequest(t, authAPIPath+"/login", map[string]string{
		"login":    "absent-user",
		"password": "password-1",
	}, nil)
	require.Equal(t, http.StatusUnauthorized, response.StatusCode)
	require.Equal(t, before, testEnv.remotes.session.sessionCount())

	_ = login(t, "auth-register", "password-1")
}

func TestAuthHTTPLogout(t *testing.T) {
	registerUser(t, "auth-logout", "password-1")
	cookie := login(t, "auth-logout", "password-1")
	response := authRequest(t, authAPIPath+"/logout", nil, cookie)
	require.Equal(t, http.StatusNoContent, response.StatusCode)
	require.False(t, testEnv.remotes.session.active(cookie.Value))
	cleared := response.Cookies()
	require.Len(t, cleared, 1)
	require.Empty(t, cleared[0].Value)
	require.Negative(t, cleared[0].MaxAge)

	response = authRequest(t, authAPIPath+"/logout", nil, cookie)
	require.Equal(t, http.StatusNoContent, response.StatusCode)
	response = authRequest(t, authAPIPath+"/logout", nil, nil)
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
}

func TestAuthHTTPChangePassword(t *testing.T) {
	registerUser(t, "auth-password", "password-1")
	current := login(t, "auth-password", "password-1")
	other := login(t, "auth-password", "password-1")

	response := authRequest(t, authAPIPath+"/change-password", map[string]string{
		"oldPassword": "password-1",
		"newPassword": "password-2",
	}, current)
	require.Equal(t, http.StatusNoContent, response.StatusCode)
	require.True(t, testEnv.remotes.session.active(current.Value))
	require.False(t, testEnv.remotes.session.active(other.Value))

	before := testEnv.remotes.session.sessionCount()
	response = authRequest(t, authAPIPath+"/login", map[string]string{
		"login":    "auth-password",
		"password": "password-1",
	}, nil)
	require.Equal(t, http.StatusUnauthorized, response.StatusCode)
	require.Equal(t, before, testEnv.remotes.session.sessionCount())
	_ = login(t, "auth-password", "password-2")

	response = authRequest(t, authAPIPath+"/change-password", map[string]string{
		"oldPassword": "password-2",
		"newPassword": "password-3",
	}, nil)
	require.Equal(t, http.StatusUnauthorized, response.StatusCode)

	response = authRequest(t, authAPIPath+"/change-password", map[string]string{
		"oldPassword": "password-2",
		"newPassword": "password-3",
	}, &http.Cookie{Name: "session", Value: "revoked"})
	require.Equal(t, http.StatusUnauthorized, response.StatusCode)

	valid := login(t, "auth-password", "password-2")
	response = authRequest(t, authAPIPath+"/change-password", map[string]string{
		"oldPassword": "wrong-password",
		"newPassword": "password-3",
	}, valid)
	require.Equal(t, http.StatusUnauthorized, response.StatusCode)
	_ = login(t, "auth-password", "password-2")

	response = authRequest(t, authAPIPath+"/change-password", map[string]string{
		"oldPassword": "password-2",
		"newPassword": "password-2",
	}, valid)
	require.Equal(t, http.StatusConflict, response.StatusCode)
}

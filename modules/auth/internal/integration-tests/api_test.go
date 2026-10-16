package integration_tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	authAPIPath       = "/api/public/v1/auth"
	initialPassword   = "password-1"
	changedPassword   = "password-2"
	thirdPassword     = "password-3"
	incorrectPassword = "wrong-password"
)

type testUser struct {
	firstName string
	lastName  string
	login     string
	email     string
	password  string
}

var testUsers = struct {
	register testUser
	logout   testUser
	password testUser
}{
	register: testUser{
		firstName: "Ada",
		lastName:  "Lovelace",
		login:     "auth-register",
		email:     "auth-register@example.com",
		password:  initialPassword,
	},
	logout: testUser{
		firstName: "Grace",
		lastName:  "Hopper",
		login:     "auth-logout",
		email:     "auth-logout@example.com",
		password:  initialPassword,
	},
	password: testUser{
		firstName: "Linus",
		lastName:  "Torvalds",
		login:     "auth-password",
		email:     "auth-password@example.com",
		password:  initialPassword,
	},
}

type registerResponse struct {
	UserID string `json:"userId"`
}

type registerRequest struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Login     string `json:"login"`
	Email     string `json:"email"`
	Password  string `json:"password"`
}

type loginRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type changePasswordRequest struct {
	OldPassword string `json:"oldPassword"`
	NewPassword string `json:"newPassword"`
}

type httpResult struct {
	statusCode int
	cookies    []*http.Cookie
	body       []byte
}

func authRequest(t *testing.T, path string, body any, cookie *http.Cookie) httpResult {
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
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	return httpResult{
		statusCode: response.StatusCode,
		cookies:    response.Cookies(),
		body:       responseBody,
	}
}

func registerUser(t *testing.T, user testUser) string {
	t.Helper()
	response := authRequest(t, authAPIPath+"/register", registerRequestFromUser(user), nil)
	require.Equal(t, http.StatusCreated, response.statusCode)
	var result registerResponse
	require.NoError(t, json.Unmarshal(response.body, &result))
	require.NotEmpty(t, result.UserID)
	return result.UserID
}

func login(t *testing.T, user testUser, password string) *http.Cookie {
	t.Helper()
	response := authRequest(t, authAPIPath+"/login", loginRequest{Login: user.login, Password: password}, nil)
	require.Equal(t, http.StatusNoContent, response.statusCode)
	cookies := response.cookies
	require.Len(t, cookies, 1)
	require.Equal(t, "session", cookies[0].Name)
	require.True(t, cookies[0].HttpOnly)
	return cookies[0]
}

func TestAuthHTTPRegisterAndLogin(t *testing.T) {
	userID := registerUser(t, testUsers.register)
	var passwordHash string
	err := testEnv.connector.TransactionalClient().GetContext(t.Context(), "test.auth.credentials", &passwordHash, `
SELECT password_hash FROM password_credentials WHERE user_id = ?`, userID)
	require.NoError(t, err)
	require.NotEmpty(t, passwordHash)

	duplicate := testUsers.register
	duplicate.email = "second@example.com"
	response := authRequest(t, authAPIPath+"/register", registerRequestFromUser(duplicate), nil)
	require.Equal(t, http.StatusConflict, response.statusCode)

	invalid := testUsers.register
	invalid.login = "invalid-email"
	invalid.email = "not-an-email"
	response = authRequest(t, authAPIPath+"/register", registerRequestFromUser(invalid), nil)
	require.Equal(t, http.StatusBadRequest, response.statusCode)

	before := testEnv.remotes.session.sessionCount()
	response = authRequest(t, authAPIPath+"/login", loginRequest{Login: testUsers.register.login, Password: incorrectPassword}, nil)
	require.Equal(t, http.StatusUnauthorized, response.statusCode)
	require.Equal(t, before, testEnv.remotes.session.sessionCount())

	response = authRequest(t, authAPIPath+"/login", loginRequest{Login: "absent-user", Password: testUsers.register.password}, nil)
	require.Equal(t, http.StatusUnauthorized, response.statusCode)
	require.Equal(t, before, testEnv.remotes.session.sessionCount())

	_ = login(t, testUsers.register, testUsers.register.password)
}

func TestAuthHTTPLogout(t *testing.T) {
	registerUser(t, testUsers.logout)
	cookie := login(t, testUsers.logout, testUsers.logout.password)
	response := authRequest(t, authAPIPath+"/logout", nil, cookie)
	require.Equal(t, http.StatusNoContent, response.statusCode)
	require.False(t, testEnv.remotes.session.active(cookie.Value))
	cleared := response.cookies
	require.Len(t, cleared, 1)
	require.Empty(t, cleared[0].Value)
	require.Negative(t, cleared[0].MaxAge)

	response = authRequest(t, authAPIPath+"/logout", nil, cookie)
	require.Equal(t, http.StatusNoContent, response.statusCode)
	response = authRequest(t, authAPIPath+"/logout", nil, nil)
	require.Equal(t, http.StatusBadRequest, response.statusCode)
}

func TestAuthHTTPChangePassword(t *testing.T) {
	registerUser(t, testUsers.password)
	current := login(t, testUsers.password, testUsers.password.password)
	other := login(t, testUsers.password, testUsers.password.password)

	response := authRequest(t, authAPIPath+"/change-password", changePasswordRequest{
		OldPassword: testUsers.password.password,
		NewPassword: changedPassword,
	}, current)
	require.Equal(t, http.StatusNoContent, response.statusCode)
	require.True(t, testEnv.remotes.session.active(current.Value))
	require.False(t, testEnv.remotes.session.active(other.Value))

	before := testEnv.remotes.session.sessionCount()
	response = authRequest(t, authAPIPath+"/login", loginRequest{
		Login:    testUsers.password.login,
		Password: testUsers.password.password,
	}, nil)
	require.Equal(t, http.StatusUnauthorized, response.statusCode)
	require.Equal(t, before, testEnv.remotes.session.sessionCount())
	_ = login(t, testUsers.password, changedPassword)

	response = authRequest(t, authAPIPath+"/change-password", changePasswordRequest{
		OldPassword: changedPassword,
		NewPassword: thirdPassword,
	}, nil)
	require.Equal(t, http.StatusUnauthorized, response.statusCode)

	response = authRequest(t, authAPIPath+"/change-password", changePasswordRequest{
		OldPassword: changedPassword,
		NewPassword: thirdPassword,
	}, &http.Cookie{
		Name:     "session",
		Value:    "revoked",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})
	require.Equal(t, http.StatusUnauthorized, response.statusCode)

	valid := login(t, testUsers.password, changedPassword)
	response = authRequest(t, authAPIPath+"/change-password", changePasswordRequest{
		OldPassword: incorrectPassword,
		NewPassword: thirdPassword,
	}, valid)
	require.Equal(t, http.StatusUnauthorized, response.statusCode)
	_ = login(t, testUsers.password, changedPassword)

	response = authRequest(t, authAPIPath+"/change-password", changePasswordRequest{
		OldPassword: changedPassword,
		NewPassword: changedPassword,
	}, valid)
	require.Equal(t, http.StatusConflict, response.statusCode)
}

func registerRequestFromUser(user testUser) registerRequest {
	return registerRequest{
		FirstName: user.firstName,
		LastName:  user.lastName,
		Login:     user.login,
		Email:     user.email,
		Password:  user.password,
	}
}

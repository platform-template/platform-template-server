package public

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"uuid"

	"github.com/stretchr/testify/require"

	sessiongrpc "notifications/internal/notifications/infra/grpc/session"
)

type fakeSessionResolver struct {
	userID uuid.UUID
	err    error
}

func (s fakeSessionResolver) Resolve(_ context.Context, _ string) (uuid.UUID, error) {
	return s.userID, s.err
}

func TestAuthenticationRejectsUnavailableSessionService(t *testing.T) {
	resolver := fakeSessionResolver{err: errors.New("unavailable")}
	called := false
	next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })
	handler := authenticateNotifications(resolver)(next)
	request := httptest.NewRequest(http.MethodGet, "/api/public/v1/notifications", http.NoBody)
	request.Header.Set("Cookie", "session=token")
	request.Header.Set("X-User-ID", uuid.New().String())
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	require.Equal(t, http.StatusServiceUnavailable, response.Code)
	require.False(t, called)
}

func TestAuthenticationRejectsInvalidSession(t *testing.T) {
	resolver := fakeSessionResolver{err: sessiongrpc.ErrInvalidSession}
	handler := authenticateNotifications(resolver)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler must not be called")
	}))
	request := httptest.NewRequest(http.MethodGet, "/api/public/v1/notifications", http.NoBody)
	request.Header.Set("Cookie", "session=revoked")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	require.Equal(t, http.StatusUnauthorized, response.Code)
}

func TestAuthenticationRejectsCrossOriginPatch(t *testing.T) {
	resolver := fakeSessionResolver{userID: uuid.New()}
	handler := authenticateNotifications(resolver)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler must not be called")
	}))
	request := httptest.NewRequest(
		http.MethodPatch,
		"http://example.test/api/public/v1/notifications/id/read",
		http.NoBody,
	)
	request.Header.Set("Cookie", "session=token")
	request.Header.Set("Origin", "https://attacker.test")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	require.Equal(t, http.StatusForbidden, response.Code)
}

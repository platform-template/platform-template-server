package integration_tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"uuid"

	sessionapi "session/api/server/internalapi"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"notifications/internal/notifications/app"
	"notifications/internal/notifications/domain"
	nmysql "notifications/internal/notifications/infra/mysql"
)

type httpResult struct {
	status int
	body   []byte
}

const accessGrantedKind = "access.granted"

func newSession(t *testing.T, userID uuid.UUID) (sessionClient sessionapi.SessionServiceClient, token string) {
	t.Helper()
	connection, err := grpc.NewClient(testEnv.sessionAddress, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	client := sessionapi.NewSessionServiceClient(connection)
	response, err := client.CreateSession(t.Context(), &sessionapi.CreateSessionRequest{UserId: userID.String()})
	require.NoError(t, err)
	return client, response.GetToken()
}

func TestNotificationsHTTP(t *testing.T) {
	user, other := uuid.New(), uuid.New()
	sessionClient, token := newSession(t, user)
	_, otherToken := newSession(t, other)
	unit := uow.NewUnitOfWork(
		mysql.NewConnectionPool(testEnv.connector.TransactionalClient()),
		nmysql.NewRepositoryProvider,
	)
	svc := app.NewService(unit, nil)

	first, err := svc.CreateNotification(t.Context(), app.CreateNotificationInput{
		UserID: user,
		Kind:   accessGrantedKind,
		Title:  "first",
	})
	require.NoError(t, err)
	second, err := svc.CreateNotification(t.Context(), app.CreateNotificationInput{
		UserID: user,
		Kind:   accessGrantedKind,
		Title:  "second",
	})
	require.NoError(t, err)
	foreign, err := svc.CreateNotification(t.Context(), app.CreateNotificationInput{
		UserID: other,
		Kind:   accessGrantedKind,
		Title:  "foreign",
	})
	require.NoError(t, err)
	olderID := first.Data().ID.String()
	if first.Data().CreatedAt.Equal(second.Data().CreatedAt) && second.Data().ID.String() < olderID {
		olderID = second.Data().ID.String()
	}

	request := func(method, path, session string, body []byte) httpResult {
		t.Helper()
		var reader io.Reader
		if body != nil {
			reader = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(t.Context(), method, "http://"+testEnv.conf.HTTP.Address+path, reader)
		require.NoError(t, err)
		if session != "" {
			req.Header.Set("Cookie", "session="+session)
		}
		req.Header.Set("X-User-ID", other.String())
		req.Header.Set("X-Actor-ID", other.String())
		req.Header.Set("X-Authenticated-User", other.String())
		response, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer response.Body.Close()
		content, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		return httpResult{status: response.StatusCode, body: content}
	}
	base := "/api/public/v1/notifications"

	response := request(http.MethodGet, base, "", nil)
	require.Equal(t, http.StatusUnauthorized, response.status)
	response = request(http.MethodGet, base, "invalid", nil)
	require.Equal(t, http.StatusUnauthorized, response.status)
	response = request(http.MethodGet, base+"?limit=1&offset=1", token, nil)
	require.Equal(t, http.StatusOK, response.status)
	var page struct {
		Notifications []struct {
			ID string `json:"id"`
		} `json:"notifications"`
	}
	require.NoError(t, json.Unmarshal(response.body, &page))
	require.Len(t, page.Notifications, 1)
	require.Equal(t, olderID, page.Notifications[0].ID)

	for _, query := range []string{"?limit=0", "?limit=1001", "?offset=-1", "?unreadOnly=bad"} {
		response = request(http.MethodGet, base+query, token, nil)
		require.Equal(t, http.StatusBadRequest, response.status)
	}
	response = request(http.MethodPatch, base+"/bad/read", token, nil)
	require.Equal(t, http.StatusBadRequest, response.status)
	response = request(http.MethodPatch, base+"/"+foreign.Data().ID.String()+"/read", token, nil)
	require.Equal(t, http.StatusNotFound, response.status)
	response = request(http.MethodPatch, base+"/"+uuid.New().String()+"/read", token, nil)
	require.Equal(t, http.StatusNotFound, response.status)
	readPath := base + "/" + first.Data().ID.String() + "/read"
	response = request(http.MethodPatch, readPath, token, []byte(`{"readAt":"2000-01-01T00:00:00Z"}`))
	require.Equal(t, http.StatusBadRequest, response.status)

	response = request(http.MethodPatch, readPath, token, nil)
	require.Equal(t, http.StatusNoContent, response.status)
	repo := nmysql.NewRepositoryProvider(testEnv.connector.TransactionalClient()).Notification(t.Context())
	stored, err := repo.GetByID(user, first.Data().ID)
	require.NoError(t, err)
	readAt := *stored.Data().ReadAt
	response = request(http.MethodPatch, readPath, token, nil)
	require.Equal(t, http.StatusNoContent, response.status)
	stored, err = repo.GetByID(user, first.Data().ID)
	require.NoError(t, err)
	require.Equal(t, readAt, *stored.Data().ReadAt)

	response = request(http.MethodGet, base+"?unreadOnly=true", token, nil)
	require.Equal(t, http.StatusOK, response.status)
	require.NoError(t, json.Unmarshal(response.body, &page))
	require.Len(t, page.Notifications, 1)
	require.Equal(t, second.Data().ID.String(), page.Notifications[0].ID)
	response = request(http.MethodGet, base+"/unread-count", token, nil)
	require.Equal(t, http.StatusOK, response.status)
	var count struct {
		Count int `json:"count"`
	}
	require.NoError(t, json.Unmarshal(response.body, &count))
	require.Equal(t, 1, count.Count)
	response = request(http.MethodGet, base+"?offset=100", token, nil)
	require.Equal(t, http.StatusOK, response.status)
	require.JSONEq(t, `{"notifications":[]}`, string(response.body))
	response = request(http.MethodGet, base, otherToken, nil)
	require.Equal(t, http.StatusOK, response.status)
	require.NoError(t, json.Unmarshal(response.body, &page))
	require.Len(t, page.Notifications, 1)
	require.Equal(t, foreign.Data().ID.String(), page.Notifications[0].ID)

	key := "source-event"
	input := app.CreateNotificationInput{
		UserID:         user,
		Kind:           "system.announcement",
		Title:          "idempotent",
		IdempotencyKey: &key,
	}
	_, err = svc.CreateNotification(t.Context(), input)
	require.NoError(t, err)
	_, err = svc.CreateNotification(t.Context(), input)
	require.ErrorIs(t, err, domain.ErrDuplicate)

	_, err = sessionClient.RevokeSession(context.Background(), &sessionapi.RevokeSessionRequest{Token: token})
	require.NoError(t, err)
	response = request(http.MethodGet, base, token, nil)
	require.Equal(t, http.StatusUnauthorized, response.status)
}

func TestApplicationDefaultLimit(t *testing.T) {
	userID := uuid.New()
	unit := uow.NewUnitOfWork(
		mysql.NewConnectionPool(testEnv.connector.TransactionalClient()),
		nmysql.NewRepositoryProvider,
	)
	svc := app.NewService(unit, nil)
	for range app.DefaultLimit + 1 {
		_, err := svc.CreateNotification(t.Context(), app.CreateNotificationInput{
			UserID: userID,
			Kind:   "system.announcement",
			Title:  "Notification",
		})
		require.NoError(t, err)
	}

	notifications, err := svc.ListNotifications(t.Context(), userID, domain.ListOptions{})
	require.NoError(t, err)
	require.Len(t, notifications, app.DefaultLimit)

	notifications, err = svc.ListNotifications(t.Context(), uuid.New(), domain.ListOptions{})
	require.NoError(t, err)
	require.Empty(t, notifications)
	require.NotNil(t, notifications)
}

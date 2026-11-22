package persistence_tests

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
	"uuid"

	authzapi "authz/api/server/internalapi"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"audit/internal/audit/app"
	"audit/internal/audit/infra/handlers/public"
	auditmysql "audit/internal/audit/infra/mysql"
)

type testAuthz struct{ authzapi.AuthzServiceClient }

func (testAuthz) IsSuperuser(
	ctx context.Context,
	_ *authzapi.IsSuperuserRequest,
	_ ...grpc.CallOption,
) (*authzapi.IsSuperuserResponse, error) {
	values, _ := metadata.FromOutgoingContext(ctx)
	tokens := values.Get("session")
	if len(tokens) != 1 {
		return nil, status.Error(codes.Unauthenticated, "invalid")
	}
	switch tokens[0] {
	case "admin":
		return &authzapi.IsSuperuserResponse{IsSuperuser: true}, nil
	case "ordinary":
		return &authzapi.IsSuperuserResponse{}, nil
	case "revoked":
		return nil, status.Error(codes.Unauthenticated, "revoked")
	default:
		return nil, status.Error(codes.Unavailable, "unavailable")
	}
}

func TestAuditHTTP(t *testing.T) {
	unit := uow.NewUnitOfWork(mysql.NewConnectionPool(testClient), auditmysql.NewRepositoryProvider)
	svc := app.NewService(unit, nil)
	actor := uuid.New()
	input := app.RecordAuditEntryInput{
		EventID:    "http/" + uuid.New().String(),
		ActorID:    &actor,
		ModuleKey:  "http_test",
		Action:     "http.login",
		Metadata:   []byte(`{"ok":true}`),
		OccurredAt: time.Now(),
	}
	_, err := svc.RecordAuditEntry(t.Context(), input)
	require.NoError(t, err)
	register, err := public.Register(svc, slog.New(slog.NewTextHandler(io.Discard, nil)), testAuthz{})
	require.NoError(t, err)
	router := chi.NewRouter()
	require.NoError(t, register(router))
	server := httptest.NewServer(router)
	defer server.Close()

	request := func(path, token string) (int, []byte) {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL+path, http.NoBody)
		require.NoError(t, err)
		if token != "" {
			req.Header.Set("Cookie", "session="+token)
		}
		req.Header.Set("X-User-ID", uuid.New().String())
		response, err := server.Client().Do(req)
		require.NoError(t, err)
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		return response.StatusCode, body
	}
	base := "/api/public/v1/audit/entries"
	for _, test := range []struct {
		token  string
		status int
	}{
		{"", 401},
		{"revoked", 401},
		{"ordinary", 403},
		{"offline", 503},
	} {
		statusCode, _ := request(base, test.token)
		require.Equal(t, test.status, statusCode)
	}
	for _, query := range []string{"?actorId=bad", "?from=bad", "?limit=101", "?offset=-1"} {
		statusCode, _ := request(base+query, "admin")
		require.Equal(t, http.StatusBadRequest, statusCode)
	}
	statusCode, body := request(base+"?actorId="+actor.String()+"&limit=1", "admin")
	require.Equal(t, http.StatusOK, statusCode)
	var result struct {
		Entries []struct {
			EventID  string         `json:"eventId"`
			Metadata map[string]any `json:"metadata"`
		} `json:"entries"`
	}
	require.NoError(t, json.Unmarshal(body, &result))
	require.Len(t, result.Entries, 1)
	require.Equal(t, input.EventID, result.Entries[0].EventID)
	require.Equal(t, true, result.Entries[0].Metadata["ok"])
	statusCode, _ = request(base+"/by-event-id?eventId="+url.QueryEscape(input.EventID), "admin")
	require.Equal(t, http.StatusOK, statusCode)
	statusCode, _ = request(base+"/by-event-id?eventId=missing", "admin")
	require.Equal(t, http.StatusNotFound, statusCode)
	_, body = request(base+"?actorId="+uuid.New().String(), "admin")
	require.NoError(t, json.Unmarshal(body, &result))
	require.NotNil(t, result.Entries)
	require.Empty(t, result.Entries)
}

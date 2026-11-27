package persistence_tests

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"uuid"

	"recipes/internal/recipes/app"
	public "recipes/internal/recipes/infra/handlers/public"
	sessionapi "session/api/server/internalapi"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeSessions struct {
	sessionapi.SessionServiceClient
	owner, stranger uuid.UUID
}

func (s fakeSessions) ResolveSession(
	_ context.Context,
	req *sessionapi.ResolveSessionRequest,
	_ ...grpc.CallOption,
) (*sessionapi.ResolveSessionResponse, error) {
	switch req.Token {
	case "owner":
		return &sessionapi.ResolveSessionResponse{UserId: s.owner.String()}, nil
	case "stranger":
		return &sessionapi.ResolveSessionResponse{UserId: s.stranger.String()}, nil
	case "offline":
		return nil, status.Error(codes.Unavailable, "offline")
	default:
		return nil, status.Error(codes.Unauthenticated, "invalid")
	}
}
func testRecipeHTTP(t *testing.T, service *app.Service, product uuid.UUID) {
	t.Helper()
	sessions := fakeSessions{owner: uuid.New(), stranger: uuid.New()}
	register, err := public.Register(service, slog.Default(), sessions)
	require.NoError(t, err)
	router := chi.NewRouter()
	require.NoError(t, register(router))
	request := func(method, path, token, body, origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		if token != "" {
			req.Header.Set("Cookie", "session="+token)
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		result := httptest.NewRecorder()
		router.ServeHTTP(result, req)
		return result
	}
	base := "/api/public/v1/recipes"
	body := fmt.Sprintf(
		`{"name":"Soup","description":"","instructions":"Cook","servings":2,"ingredients":[{"productId":%q,"quantity":5}]}`,
		product.String(),
	)
	require.Equal(t, http.StatusUnauthorized, request("GET", base, "", "", "").Code)
	require.Equal(t, http.StatusServiceUnavailable, request("GET", base, "offline", "", "").Code)
	require.Equal(t, http.StatusForbidden, request("POST", base, "owner", body, "https://evil.example").Code)
	created := request("POST", base, "owner", body, "")
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	var item struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(created.Body.Bytes(), &item))
	require.NotEmpty(t, item.ID)
	path := base + "/" + item.ID
	require.Equal(t, http.StatusNotFound, request("GET", path, "stranger", "", "").Code)
	require.Equal(t, http.StatusOK, request("GET", path, "owner", "", "").Code)
	scaled := request("GET", path+"/ingredients?servings=3", "owner", "", "")
	require.Equal(t, http.StatusOK, scaled.Code, scaled.Body.String())
	require.Contains(t, scaled.Body.String(), `"quantity":8`)
	require.Equal(t, http.StatusNoContent, request("PUT", path, "owner", body, "").Code)
	require.Equal(t, http.StatusNotFound, request("DELETE", path, "stranger", "", "").Code)
	require.Equal(t, http.StatusNoContent, request("DELETE", path, "owner", "", "").Code)
	require.Equal(t, http.StatusNotFound, request("GET", path, "owner", "", "").Code)
}

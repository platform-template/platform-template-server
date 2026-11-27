package persistence_tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"uuid"

	authzapi "authz/api/server/internalapi"
	sessionapi "session/api/server/internalapi"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"inventory/internal/inventory/infra/handlers/public"
)

type fakeSessions struct {
	sessionapi.SessionServiceClient
}

var httpUserID = uuid.New()
var httpAdminID = uuid.New()

func (fakeSessions) ResolveSession(
	_ context.Context,
	req *sessionapi.ResolveSessionRequest,
	_ ...grpc.CallOption,
) (*sessionapi.ResolveSessionResponse, error) {
	switch req.GetToken() {
	case "admin":
		return &sessionapi.ResolveSessionResponse{UserId: httpAdminID.String()}, nil
	case "user", "authz-offline":
		return &sessionapi.ResolveSessionResponse{UserId: httpUserID.String()}, nil
	case "revoked":
		return nil, status.Error(codes.Unauthenticated, "invalid")
	default:
		return nil, status.Error(codes.Unavailable, "unavailable")
	}
}

type fakeAuthz struct{ authzapi.AuthzServiceClient }

func (fakeAuthz) IsSuperuser(
	ctx context.Context,
	_ *authzapi.IsSuperuserRequest,
	_ ...grpc.CallOption,
) (*authzapi.IsSuperuserResponse, error) {
	md, _ := metadata.FromOutgoingContext(ctx)
	tokens := md.Get("session")
	if len(tokens) != 1 || tokens[0] == "authz-offline" {
		return nil, status.Error(codes.Unavailable, "unavailable")
	}
	return &authzapi.IsSuperuserResponse{IsSuperuser: tokens[0] == "admin"}, nil
}

func TestProductHTTP(t *testing.T) {
	register, err := public.Register(
		newService(),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		fakeSessions{},
		fakeAuthz{},
	)
	require.NoError(t, err)
	router := chi.NewRouter()
	require.NoError(t, register(router))
	server := httptest.NewServer(router)
	defer server.Close()
	request := func(method, path, token, body, origin string) (int, []byte) {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), method, server.URL+path, bytes.NewBufferString(body))
		require.NoError(t, err)
		if token != "" {
			req.Header.Set("Cookie", "session="+token)
		}
		if body != "" {
			req.Header.Set("Content-Type", "application/json")
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		req.Header.Set("X-User-ID", uuid.New().String())
		resp, err := server.Client().Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp.StatusCode, data
	}
	base := "/api/public/v1/products"
	code, _ := request(http.MethodGet, base, "", "", "")
	require.Equal(t, 401, code)
	code, _ = request(http.MethodGet, base, "revoked", "", "")
	require.Equal(t, 401, code)
	code, _ = request(http.MethodGet, base, "session-offline", "", "")
	require.Equal(t, 503, code)
	code, _ = request(http.MethodPost, base, "user", `{"name":"milk","unit":"ml"}`, "")
	require.Equal(t, 403, code)
	code, _ = request(http.MethodPost, base, "authz-offline", `{"name":"milk","unit":"ml"}`, "")
	require.Equal(t, 503, code)
	code, _ = request(http.MethodPost, base, "admin", `{"name":"milk","unit":"ml"}`, "https://evil.example")
	require.Equal(t, 403, code)
	code, body := request(http.MethodPost, base, "admin", `{"name":"Молоко","unit":"ml"}`, "")
	require.Equal(t, 201, code)
	var product struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(body, &product))
	require.NotEmpty(t, product.ID)
	code, _ = request(http.MethodGet, base+"/"+product.ID, "user", "", "")
	require.Equal(t, 200, code)
	code, body = request(http.MethodPost, "/api/public/v1/inventories", "user", `{"name":"Холодильник"}`, "")
	require.Equal(t, 201, code)
	var inventory struct {
		ID string `json:"id"`
	}
	require.NoError(t, json.Unmarshal(body, &inventory))
	stockPath := "/api/public/v1/inventories/" + inventory.ID + "/stocks/" + product.ID
	code, _ = request(http.MethodGet, stockPath, "user", "", "")
	require.Equal(t, 200, code)
	code, _ = request(http.MethodPost, stockPath+"/add", "admin", `{"quantity":5}`, "")
	require.Equal(t, 404, code)
	code, _ = request(http.MethodPost, stockPath+"/add", "user", `{"quantity":5}`, "")
	require.Equal(t, 204, code)
	code, body = request(http.MethodGet, stockPath, "user", "", "")
	require.Equal(t, 200, code)
	var stock struct {
		Quantity  int64 `json:"quantity"`
		Available int64 `json:"available"`
	}
	require.NoError(t, json.Unmarshal(body, &stock))
	require.EqualValues(t, 5, stock.Quantity)
	require.EqualValues(t, 5, stock.Available)
	code, _ = request(http.MethodPost, stockPath+"/add", "user", `{"quantity":0}`, "")
	require.Equal(t, 400, code)
	code, _ = request(http.MethodGet, base+"/bad", "user", "", "")
	require.Equal(t, 400, code)
	code, _ = request(http.MethodGet, base+"/"+uuid.New().String(), "user", "", "")
	require.Equal(t, 404, code)
	code, body = request(http.MethodGet, base+"?limit=1&offset=0", "user", "", "")
	require.Equal(t, 200, code)
	var list struct {
		Products []json.RawMessage `json:"products"`
	}
	require.NoError(t, json.Unmarshal(body, &list))
	require.Len(t, list.Products, 1)
	code, body = request(http.MethodGet, base+"?search=not-present-unique", "user", "", "")
	require.Equal(t, 200, code)
	require.NoError(t, json.Unmarshal(body, &list))
	require.NotNil(t, list.Products)
	require.Empty(t, list.Products)
}

//go:build e2e

package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	authzapi "authz/api/server/internalapi"
)

func TestAuthzGateway(t *testing.T) {
	ctx, client, admin := scenario(t)
	adminID := registeredUser(t, ctx, client, admin)
	require.NoError(t, env.BootstrapSuperuser(ctx, adminID))
	adminCookie := loggedIn(t, ctx, client, admin.Login, admin.Password)
	_, _, user := scenario(t)
	userID := registeredUser(t, ctx, client, user)
	userCookie := loggedIn(t, ctx, client, user.Login, user.Password)
	address, err := env.GatewayAddress(ctx)
	require.NoError(t, err)
	conn, err := env.GRPC(ctx, "authz")
	require.NoError(t, err)
	grpcClient := authzapi.NewAuthzServiceClient(conn)
	userCtx := metadata.NewOutgoingContext(ctx, metadata.Pairs("session", userCookie.Value))
	adminCtx := metadata.NewOutgoingContext(ctx, metadata.Pairs("session", adminCookie.Value))
	list := func(ctx context.Context, id string) *authzapi.ListAccessResponse {
		result, err := grpcClient.ListAccess(ctx, &authzapi.ListAccessRequest{UserId: id})
		require.NoError(t, err)
		return result
	}
	call := func(method, path string, cookie *http.Cookie, body any) int {
		var encoded bytes.Buffer
		if body != nil {
			require.NoError(t, json.NewEncoder(&encoded).Encode(body))
		}
		request, err := http.NewRequestWithContext(ctx, method, address+path, &encoded)
		require.NoError(t, err)
		if body != nil {
			request.Header.Set("Content-Type", "application/json")
		}
		if method == http.MethodPost || method == http.MethodDelete {
			request.Header.Set("Origin", address)
		}
		if cookie != nil {
			request.AddCookie(cookie)
		}
		response, err := http.DefaultClient.Do(request)
		require.NoError(t, err)
		defer response.Body.Close()
		return response.StatusCode
	}
	statusCode := call(http.MethodGet, "/test/inventory", nil, nil)
	require.Equal(t, 401, statusCode)
	statusCode = call(http.MethodGet, "/test/inventory", &http.Cookie{Name: "session", Value: "invalid"}, nil)
	require.Equal(t, 401, statusCode)
	statusCode = call(http.MethodGet, "/test/inventory", userCookie, nil)
	require.Equal(t, 403, statusCode)
	statusCode = call(http.MethodGet, "/test/inventory", adminCookie, nil)
	require.Equal(t, http.StatusNoContent, statusCode)
	require.True(t, list(adminCtx, adminID).GetIsSuperuser())
	require.False(t, list(userCtx, userID).GetIsSuperuser())
	adminRole, err := grpcClient.IsSuperuser(adminCtx, &authzapi.IsSuperuserRequest{})
	require.NoError(t, err)
	require.True(t, adminRole.GetIsSuperuser())
	userRole, err := grpcClient.IsSuperuser(userCtx, &authzapi.IsSuperuserRequest{})
	require.NoError(t, err)
	require.False(t, userRole.GetIsSuperuser())
	_, err = grpcClient.IsSuperuser(ctx, &authzapi.IsSuperuserRequest{})
	require.Equal(t, codes.Unauthenticated, status.Code(err))
	body := map[string]string{"target_user_id": userID, "module_key": "inventory", "actor_id": adminID}
	statusCode = call(http.MethodPost, "/api/v1/authz/grants", userCookie, body)
	require.Equal(t, 403, statusCode)
	statusCode = call(http.MethodPost, "/api/v1/authz/grants", adminCookie, body)
	require.Equal(t, 204, statusCode)
	before := list(userCtx, userID)
	require.Len(t, before.GetGrants(), 1)
	require.Equal(t, adminID, before.GetGrants()[0].GetGrantedBy())
	statusCode = call(http.MethodPost, "/api/v1/authz/grants", adminCookie, body)
	require.Equal(t, 204, statusCode)
	require.Equal(t, before.GetGrants()[0].GetGrantedAt(), list(adminCtx, userID).GetGrants()[0].GetGrantedAt())
	require.Equal(t, before.GetGrants()[0].GetGrantedBy(), list(adminCtx, userID).GetGrants()[0].GetGrantedBy())
	checked, err := grpcClient.CheckAccess(
		userCtx,
		&authzapi.CheckAccessRequest{UserId: userID, ModuleKey: "inventory"},
	)
	require.NoError(t, err)
	require.True(t, checked.GetAllowed())
	_, err = grpcClient.ListAccess(userCtx, &authzapi.ListAccessRequest{UserId: adminID})
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	statusCode = call(http.MethodGet, "/test/inventory", userCookie, nil)
	require.Equal(t, http.StatusNoContent, statusCode)
	path := "/api/v1/authz/grants/" + userID + "/inventory"
	statusCode = call(http.MethodDelete, path, userCookie, nil)
	require.Equal(t, 403, statusCode)
	statusCode = call(http.MethodDelete, path, adminCookie, nil)
	require.Equal(t, 204, statusCode)
	statusCode = call(http.MethodDelete, path, adminCookie, nil)
	require.Equal(t, 204, statusCode)
	require.Empty(t, list(userCtx, userID).GetGrants())
	statusCode = call(http.MethodGet, "/test/inventory", userCookie, nil)
	require.Equal(t, 403, statusCode)
	logout, err := client.LogoutWithResponse(ctx, withCookie(userCookie))
	require.NoError(t, err)
	require.Equal(t, 204, logout.StatusCode())
	statusCode = call(http.MethodGet, "/test/inventory", userCookie, nil)
	require.Equal(t, 401, statusCode)
	for _, dependency := range []string{"session", "authz"} {
		require.NoError(t, env.StopService(ctx, dependency))
		t.Cleanup(func() { _ = env.StartService(context.Background(), dependency) })
		statusCode = call(http.MethodGet, "/test/inventory", adminCookie, nil)
		require.NotEqual(t, http.StatusNoContent, statusCode)
		require.NoError(t, env.StartService(ctx, dependency))
		require.Eventually(t, func() bool {
			code := call(http.MethodGet, "/test/inventory", adminCookie, nil)
			return code == http.StatusNoContent
		}, 20*time.Second, 250*time.Millisecond)
	}
}

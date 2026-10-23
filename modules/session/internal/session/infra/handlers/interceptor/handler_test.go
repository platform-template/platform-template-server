package interceptor

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"uuid"

	interceptorgrpc "github.com/distributed-programming-2026/lib/gateway/interceptor"
	"github.com/stretchr/testify/require"

	"session/internal/session/domain"
)

type fakeResolver struct {
	userID uuid.UUID
	err    error
	token  string
}

func (r *fakeResolver) Resolve(_ context.Context, token string) (uuid.UUID, error) {
	r.token = token
	return r.userID, r.err
}

func TestHandlerIntercept(t *testing.T) {
	userID := uuid.New()
	tests := []struct {
		name       string
		cookie     string
		resolver   fakeResolver
		statusCode int32
		wantUserID string
	}{
		{name: "valid session", cookie: "session=token", resolver: fakeResolver{userID: userID}, wantUserID: userID.String()},
		{name: "missing session", cookie: "other=value", statusCode: 401},
		{name: "expired session", cookie: "session=expired", resolver: fakeResolver{err: domain.ErrSessionExpired}, statusCode: 401},
		{name: "resolver failure", cookie: "session=token", resolver: fakeResolver{err: errors.New("database unavailable")}, statusCode: 503},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver := tt.resolver
			h := NewHandler(&resolver, slog.New(slog.NewTextHandler(io.Discard, nil)))
			response, err := h.Intercept(context.Background(), requestWithCookie(tt.cookie))
			require.NoError(t, err)

			if tt.statusCode != 0 {
				require.Equal(t, tt.statusCode, response.GetImmediateResponse().GetStatusCode())
				return
			}

			mutation := response.GetContinueResponse().GetHeaderMutation()
			require.Equal(t, "token", resolver.token)
			require.Len(t, mutation.GetSet(), 1)
			require.Equal(t, authUserHeader, mutation.GetSet()[0].GetKey())
			require.NotEmpty(t, mutation.GetSet()[0].GetValues())
			require.Equal(t, tt.wantUserID, mutation.GetSet()[0].GetValues()[0])
		})
	}
}

func requestWithCookie(cookie string) *interceptorgrpc.InterceptRequest {
	return &interceptorgrpc.InterceptRequest{
		Context: &interceptorgrpc.ProcessingContext{
			Request: &interceptorgrpc.HttpRequest{Headers: []*interceptorgrpc.Header{{Key: "Cookie", Value: cookie}}},
		},
		Event: &interceptorgrpc.InterceptRequest_RequestHeaders{RequestHeaders: &interceptorgrpc.HttpHeaders{}},
	}
}

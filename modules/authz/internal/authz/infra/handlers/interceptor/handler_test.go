package interceptor

import (
	"context"
	"errors"
	"testing"
	"uuid"

	interceptorgrpc "github.com/distributed-programming-2026/lib/gateway/interceptor"
	"github.com/stretchr/testify/require"

	sessiongrpc "authz/internal/authz/infra/grpc/session"
)

type sessions struct {
	id  uuid.UUID
	err error
}

func (s sessions) Resolve(
	context.Context,
	string,
) (uuid.UUID, error) {
	return s.id, s.err
}

type access struct {
	allow bool
	err   error
}

func (a access) CheckAccess(
	context.Context,
	uuid.UUID,
	string,
) (bool, error) {
	return a.allow, a.err
}

func TestInterceptor(t *testing.T) {
	const inventory = "inventory"
	const validCookie = "session=valid"
	for _, tc := range []struct {
		name, cookie, key string
		sessions          sessions
		access            access
		status            int32
	}{
		{name: "allow", cookie: validCookie, key: inventory, sessions: sessions{id: uuid.New()}, access: access{allow: true}},
		{name: "forbid", cookie: validCookie, key: inventory, sessions: sessions{id: uuid.New()}, status: 403},
		{name: "missing cookie", key: inventory, status: 401},
		{name: "invalid session", cookie: "session=invalid", key: inventory, sessions: sessions{err: sessiongrpc.ErrInvalidSession}, status: 401},
		{name: "session unavailable", cookie: validCookie, key: inventory, sessions: sessions{err: errors.New("unavailable")}, status: 503},
		{name: "storage unavailable", cookie: validCookie, key: inventory, sessions: sessions{id: uuid.New()}, access: access{err: errors.New("unavailable")}, status: 503},
		{name: "missing module", cookie: validCookie, status: 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := &interceptorgrpc.InterceptRequest{
				Context: &interceptorgrpc.ProcessingContext{
					Parameters: map[string]string{"module_key": tc.key},
					Request: &interceptorgrpc.HttpRequest{
						Headers: []*interceptorgrpc.Header{
							{Key: "Cookie", Value: tc.cookie},
						},
					},
				},
				Event: &interceptorgrpc.InterceptRequest_RequestHeaders{
					RequestHeaders: &interceptorgrpc.HttpHeaders{},
				},
			}
			response, err := NewHandler(
				tc.sessions,
				tc.access,
			).Intercept(t.Context(), request)
			require.NoError(t, err)
			if tc.status == 0 {
				require.NotNil(t, response.GetContinueResponse())
				return
			}
			require.Equal(
				t,
				tc.status,
				response.GetImmediateResponse().GetStatusCode(),
			)
		})
	}
}

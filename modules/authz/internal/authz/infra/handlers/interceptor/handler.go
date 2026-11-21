package interceptor

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"uuid"

	interceptorgrpc "github.com/distributed-programming-2026/lib/gateway/interceptor"

	"authz/internal/authz/domain"
	sessiongrpc "authz/internal/authz/infra/grpc/session"
)

type sessionResolver interface {
	Resolve(context.Context, string) (uuid.UUID, error)
}

type accessChecker interface {
	CheckAccess(context.Context, uuid.UUID, string) (bool, error)
}

type handler struct {
	interceptorgrpc.UnimplementedInterceptorServer
	sessions sessionResolver
	access   accessChecker
}

func NewHandler(
	sessions sessionResolver,
	access accessChecker,
) interceptorgrpc.InterceptorServer {
	return &handler{sessions: sessions, access: access}
}

func (h *handler) Intercept(
	ctx context.Context,
	req *interceptorgrpc.InterceptRequest,
) (*interceptorgrpc.InterceptResponse, error) {
	if req.GetRequestHeaders() == nil {
		return deny(http.StatusServiceUnavailable), nil
	}
	key, err := domain.NewModuleKey(
		req.GetContext().GetParameters()["module_key"],
	)
	if err != nil {
		return deny(http.StatusServiceUnavailable), nil
	}
	request := &http.Request{Header: make(http.Header)}
	for _, header := range req.GetContext().GetRequest().GetHeaders() {
		if strings.EqualFold(header.GetKey(), "Cookie") {
			request.Header.Add("Cookie", header.GetValue())
		}
	}
	cookie, err := request.Cookie("session")
	if err != nil || cookie.Value == "" {
		return deny(http.StatusUnauthorized), nil
	}
	userID, err := h.sessions.Resolve(ctx, cookie.Value)
	if errors.Is(err, sessiongrpc.ErrInvalidSession) {
		return deny(http.StatusUnauthorized), nil
	}
	if err != nil || userID == uuid.Nil() {
		return deny(http.StatusServiceUnavailable), nil
	}
	allowed, err := h.access.CheckAccess(ctx, userID, key.String())
	if err != nil {
		return deny(http.StatusServiceUnavailable), nil
	}
	if !allowed {
		return deny(http.StatusForbidden), nil
	}
	return &interceptorgrpc.InterceptResponse{
		Result: &interceptorgrpc.InterceptResponse_ContinueResponse{
			ContinueResponse: &interceptorgrpc.ContinueResponse{},
		},
	}, nil
}

func deny(code int32) *interceptorgrpc.InterceptResponse {
	return &interceptorgrpc.InterceptResponse{
		Result: &interceptorgrpc.InterceptResponse_ImmediateResponse{
			ImmediateResponse: &interceptorgrpc.ImmediateResponse{
				StatusCode: code,
			},
		},
	}
}

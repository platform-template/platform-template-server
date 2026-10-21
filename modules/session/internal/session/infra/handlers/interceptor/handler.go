package interceptor

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"uuid"

	interceptorgrpc "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
	"github.com/distributed-programming-2026/go-sdk/pkg/logging"

	"session/internal/session/domain"
)

const (
	sessionCookieName = "session"
	authUserHeader    = "X-Auth-User"
)

type handler struct {
	interceptorgrpc.UnimplementedInterceptorServer
	svc    sessionResolver
	logger *slog.Logger
}

type sessionResolver interface {
	Resolve(context.Context, string) (uuid.UUID, error)
}

func NewHandler(svc sessionResolver, logger *slog.Logger) interceptorgrpc.InterceptorServer {
	return &handler{svc: svc, logger: logger}
}

func (h *handler) Intercept(
	ctx context.Context,
	req *interceptorgrpc.InterceptRequest,
) (*interceptorgrpc.InterceptResponse, error) {
	if req.GetRequestHeaders() == nil {
		return continueResponse(), nil
	}

	token := sessionToken(req.GetContext().GetRequest().GetHeaders())
	if token == "" {
		return immediateResponse(401), nil
	}

	userID, err := h.svc.Resolve(ctx, token)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidToken),
			errors.Is(err, domain.ErrSessionNotFound),
			errors.Is(err, domain.ErrSessionExpired):
			return immediateResponse(401), nil
		default:
			h.logger.ErrorContext(ctx, "resolve session for gateway failed", logging.Error(err))
			return immediateResponse(503), nil
		}
	}

	return &interceptorgrpc.InterceptResponse{
		Result: &interceptorgrpc.InterceptResponse_ContinueResponse{
			ContinueResponse: &interceptorgrpc.ContinueResponse{
				HeaderMutation: &interceptorgrpc.HeaderMutation{
					Set: []*interceptorgrpc.HeaderValues{{
						Key:    authUserHeader,
						Values: []string{userID.String()},
					}},
				},
			},
		},
	}, nil
}

func sessionToken(headers []*interceptorgrpc.Header) string {
	for _, header := range headers {
		if !strings.EqualFold(header.GetKey(), "Cookie") {
			continue
		}

		for _, item := range strings.Split(header.GetValue(), ";") {
			name, value, found := strings.Cut(strings.TrimSpace(item), "=")
			if found && name == sessionCookieName && value != "" {
				return value
			}
		}
	}

	return ""
}

func continueResponse() *interceptorgrpc.InterceptResponse {
	return &interceptorgrpc.InterceptResponse{
		Result: &interceptorgrpc.InterceptResponse_ContinueResponse{
			ContinueResponse: &interceptorgrpc.ContinueResponse{},
		},
	}
}

func immediateResponse(statusCode int32) *interceptorgrpc.InterceptResponse {
	return &interceptorgrpc.InterceptResponse{
		Result: &interceptorgrpc.InterceptResponse_ImmediateResponse{
			ImmediateResponse: &interceptorgrpc.ImmediateResponse{StatusCode: statusCode},
		},
	}
}

package session

import (
	"context"
	"net"
	"testing"
	"uuid"

	sessionapi "session/api/server/internalapi"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type sessionServer struct {
	sessionapi.UnimplementedSessionServiceServer
	userID uuid.UUID
}

func (s sessionServer) ResolveSession(
	_ context.Context,
	request *sessionapi.ResolveSessionRequest,
) (*sessionapi.ResolveSessionResponse, error) {
	if request.Token != "active" {
		return nil, status.Error(codes.Unauthenticated, "invalid")
	}
	return &sessionapi.ResolveSessionResponse{UserId: s.userID.String()}, nil
}

func TestResolveSession(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	userID := uuid.New()
	sessionapi.RegisterSessionServiceServer(server, sessionServer{userID: userID})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)

	connection, err := grpc.NewClient(
		"passthrough:///session",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = connection.Close() })
	client := NewClient(connection)

	resolved, err := client.Resolve(t.Context(), "active")
	require.NoError(t, err)
	require.Equal(t, userID, resolved)

	_, err = client.Resolve(t.Context(), "revoked")
	require.ErrorIs(t, err, ErrInvalidSession)

	server.Stop()
	_, err = client.Resolve(t.Context(), "active")
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrInvalidSession)
}

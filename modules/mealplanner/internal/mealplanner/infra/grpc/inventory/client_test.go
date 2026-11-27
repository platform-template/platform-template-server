package inventory

import (
	"context"
	"net"
	"testing"
	"uuid"

	inventoryapi "inventory/api/server/internalapi"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type ownershipServer struct {
	inventoryapi.UnimplementedInventoryServiceServer
	owner, allowed string
}

func (s ownershipServer) IsInventoryOwner(
	_ context.Context,
	req *inventoryapi.IsInventoryOwnerRequest,
) (*inventoryapi.IsInventoryOwnerResponse, error) {
	return &inventoryapi.IsInventoryOwnerResponse{IsOwner: req.UserId == s.owner && req.InventoryId == s.allowed}, nil
}
func TestCheckOwnersOverGRPC(t *testing.T) {
	owner, inventoryID := uuid.New(), uuid.New()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	inventoryapi.RegisterInventoryServiceServer(
		server,
		ownershipServer{owner: owner.String(), allowed: inventoryID.String()},
	)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient(
		"passthrough:///inventory",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	client := NewClient(inventoryapi.NewInventoryServiceClient(conn))
	require.NoError(t, client.CheckOwners(t.Context(), owner, []uuid.UUID{inventoryID}))
	require.ErrorIs(
		t,
		client.CheckOwners(t.Context(), owner, []uuid.UUID{inventoryID, uuid.New()}),
		ErrInventoryNotAccessible,
	)
	require.ErrorIs(t, client.CheckOwners(t.Context(), uuid.New(), []uuid.UUID{inventoryID}), ErrInventoryNotAccessible)
	require.ErrorIs(t, client.CheckOwners(t.Context(), owner, nil), ErrInventoryNotAccessible)
	server.Stop()
	require.Error(t, client.CheckOwners(t.Context(), owner, []uuid.UUID{inventoryID}))
}

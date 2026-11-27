package catalog

import (
	"context"
	"net"
	"testing"
	"uuid"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	inventoryapi "inventory/api/server/internalapi"
	"recipes/internal/recipes/domain"
)

type catalogServer struct {
	inventoryapi.UnimplementedInventoryServiceServer
	productID uuid.UUID
}

func (s catalogServer) GetProduct(
	_ context.Context,
	req *inventoryapi.GetProductRequest,
) (*inventoryapi.Product, error) {
	if req.GetProductId() != s.productID.String() {
		return nil, status.Error(codes.NotFound, "missing")
	}
	return &inventoryapi.Product{Id: s.productID.String(), Name: "Греча", Unit: "g"}, nil
}

func TestCatalogGRPCContract(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	id := uuid.New()
	inventoryapi.RegisterInventoryServiceServer(server, catalogServer{productID: id})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient("passthrough:///inventory",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, conn.Close()) })
	client := NewClient(inventoryapi.NewInventoryServiceClient(conn))
	require.NoError(t, client.ValidateProduct(t.Context(), id))
	require.ErrorIs(t, client.ValidateProduct(t.Context(), uuid.New()), domain.ErrProductNotFound)
}

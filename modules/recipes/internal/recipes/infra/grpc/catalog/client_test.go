package catalog

import (
	"context"
	"testing"
	"uuid"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	inventoryapi "inventory/api/server/internalapi"
	"recipes/internal/recipes/domain"
)

type fakeInventory struct {
	inventoryapi.InventoryServiceClient
	response *inventoryapi.Product
	err      error
}

func (f fakeInventory) GetProduct(
	_ context.Context,
	_ *inventoryapi.GetProductRequest,
	_ ...grpc.CallOption,
) (*inventoryapi.Product, error) {
	return f.response, f.err
}

func TestValidateProduct(t *testing.T) {
	id := uuid.New()
	for _, test := range []struct {
		name   string
		client fakeInventory
		want   error
	}{
		{name: "exists", client: fakeInventory{response: &inventoryapi.Product{Id: id.String()}}},
		{name: "missing", client: fakeInventory{err: status.Error(codes.NotFound, "missing")}, want: domain.ErrProductNotFound},
		{name: "unavailable", client: fakeInventory{err: status.Error(codes.Unavailable, "offline")}, want: status.Error(codes.Unavailable, "offline")},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := NewClient(test.client).ValidateProduct(t.Context(), id)
			if test.want == nil {
				require.NoError(t, err)
				return
			}
			if test.want == domain.ErrProductNotFound {
				require.ErrorIs(t, err, test.want)
				return
			}
			require.Equal(t, codes.Unavailable, status.Code(err))
		})
	}
	require.Error(
		t,
		NewClient(
			fakeInventory{response: &inventoryapi.Product{Id: uuid.New().String()}},
		).ValidateProduct(t.Context(), id),
	)
}

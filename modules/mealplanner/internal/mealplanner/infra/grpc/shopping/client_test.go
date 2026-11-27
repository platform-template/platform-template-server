package shopping

import (
	"context"
	"errors"
	"testing"
	"uuid"

	shoppingapi "shopping/api/server/internalapi"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type fakeShopping struct {
	shoppingapi.ShoppingServiceClient
	response *shoppingapi.IsListOwnerResponse
	err      error
	request  *shoppingapi.IsListOwnerRequest
}

func (f *fakeShopping) IsListOwner(
	_ context.Context,
	req *shoppingapi.IsListOwnerRequest,
	_ ...grpc.CallOption,
) (*shoppingapi.IsListOwnerResponse, error) {
	f.request = req
	return f.response, f.err
}
func TestCheckOwner(t *testing.T) {
	owner, list := uuid.New(), uuid.New()
	api := &fakeShopping{response: &shoppingapi.IsListOwnerResponse{IsOwner: true}}
	client := NewClient(api)
	require.NoError(t, client.CheckOwner(t.Context(), owner, list))
	require.Equal(t, owner.String(), api.request.GetUserId())
	require.Equal(t, list.String(), api.request.GetListId())
	api.response.IsOwner = false
	require.ErrorIs(t, client.CheckOwner(t.Context(), owner, list), ErrListNotAccessible)
	api.response = nil
	require.ErrorIs(t, client.CheckOwner(t.Context(), owner, list), ErrListNotAccessible)
	failure := errors.New("unavailable")
	api.err = failure
	require.ErrorIs(t, client.CheckOwner(t.Context(), owner, list), failure)
	require.ErrorIs(t, client.CheckOwner(t.Context(), uuid.Nil(), list), ErrListNotAccessible)
}

package shopping

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	shoppingapi "shopping/api/server/internalapi"
)

var ErrListNotAccessible = errors.New("shopping list not accessible")

type Client struct {
	api shoppingapi.ShoppingServiceClient
}

func NewClient(api shoppingapi.ShoppingServiceClient) *Client { return &Client{api: api} }
func (c *Client) CheckOwner(ctx context.Context, userID, listID uuid.UUID) error {
	if userID == uuid.Nil() || listID == uuid.Nil() {
		return ErrListNotAccessible
	}
	response, err := c.api.IsListOwner(
		ctx,
		&shoppingapi.IsListOwnerRequest{UserId: userID.String(), ListId: listID.String()},
	)
	if err != nil {
		return fmt.Errorf("check shopping list ownership: %w", err)
	}
	if response == nil || !response.GetIsOwner() {
		return ErrListNotAccessible
	}
	return nil
}

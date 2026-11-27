package inventory

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	inventoryapi "inventory/api/server/internalapi"
)

var ErrInventoryNotAccessible = errors.New("inventory not accessible")

type Client struct {
	api inventoryapi.InventoryServiceClient
}

func NewClient(api inventoryapi.InventoryServiceClient) *Client { return &Client{api: api} }

// CheckOwners fails closed if any chosen inventory is inaccessible or cannot be checked.
func (c *Client) CheckOwners(ctx context.Context, userID uuid.UUID, inventories []uuid.UUID) error {
	if userID == uuid.Nil() || len(inventories) == 0 {
		return ErrInventoryNotAccessible
	}
	for _, inventoryID := range inventories {
		if inventoryID == uuid.Nil() {
			return ErrInventoryNotAccessible
		}
		result, err := c.api.IsInventoryOwner(
			ctx,
			&inventoryapi.IsInventoryOwnerRequest{UserId: userID.String(), InventoryId: inventoryID.String()},
		)
		if err != nil {
			return fmt.Errorf("check inventory ownership: %w", err)
		}
		if result == nil || !result.GetIsOwner() {
			return ErrInventoryNotAccessible
		}
	}
	return nil
}

func (c *Client) ValidateProduct(ctx context.Context, productID uuid.UUID) error {
	if productID == uuid.Nil() {
		return errors.New("invalid product ID")
	}
	response, err := c.api.GetProduct(ctx, &inventoryapi.GetProductRequest{ProductId: productID.String()})
	if err != nil {
		return fmt.Errorf("get inventory product: %w", err)
	}
	if response == nil || response.GetId() != productID.String() {
		return errors.New("invalid product response")
	}
	return nil
}

package catalog

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	inventoryapi "inventory/api/server/internalapi"
	"recipes/internal/recipes/domain"
)

type Client struct {
	client inventoryapi.InventoryServiceClient
}

func NewClient(client inventoryapi.InventoryServiceClient) *Client { return &Client{client: client} }

func (c *Client) ValidateProduct(ctx context.Context, id uuid.UUID) error {
	response, err := c.client.GetProduct(ctx, &inventoryapi.GetProductRequest{ProductId: id.String()})
	if status.Code(err) == codes.NotFound {
		return domain.ErrProductNotFound
	}
	if err != nil {
		return fmt.Errorf("get catalog product: %w", err)
	}
	if response == nil || response.GetId() != id.String() {
		return errors.New("invalid catalog response")
	}
	return nil
}

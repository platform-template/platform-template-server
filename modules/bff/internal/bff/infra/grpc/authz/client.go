package authz

import (
	"context"
	"fmt"
	"uuid"

	authzapi "authz/api/server/internalapi"

	"google.golang.org/grpc/metadata"

	"bff/internal/bff/app"
)

type Client struct{ api authzapi.AuthzServiceClient }

func NewClient(api authzapi.AuthzServiceClient) *Client { return &Client{api: api} }

func (c *Client) List(
	ctx context.Context,
	token string,
	userID uuid.UUID,
) (modules []string, superuser bool, err error) {
	ctx = metadata.NewOutgoingContext(ctx, metadata.Pairs("session", token))
	response, err := c.api.ListAccess(ctx, &authzapi.ListAccessRequest{UserId: userID.String()})
	if err != nil {
		return nil, false, fmt.Errorf("list module access: %w", err)
	}
	if response == nil {
		return nil, false, app.ErrUnavailable
	}
	modules = make([]string, 0, len(response.GetGrants()))
	for _, grant := range response.GetGrants() {
		if grant.GetModuleKey() != "" {
			modules = append(modules, grant.GetModuleKey())
		}
	}
	return modules, response.GetIsSuperuser(), nil
}

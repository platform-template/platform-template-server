package session

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	sessionapi "session/api/server/internalapi"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"bff/internal/bff/app"
)

type Client struct {
	api sessionapi.SessionServiceClient
}

func NewClient(api sessionapi.SessionServiceClient) *Client { return &Client{api: api} }

func (c *Client) Resolve(ctx context.Context, token string) (uuid.UUID, error) {
	response, err := c.api.ResolveSession(ctx, &sessionapi.ResolveSessionRequest{Token: token})
	if status.Code(err) == codes.Unauthenticated || status.Code(err) == codes.InvalidArgument {
		return uuid.Nil(), app.ErrUnauthenticated
	}
	if err != nil {
		return uuid.Nil(), fmt.Errorf("resolve session: %w", err)
	}
	if response == nil {
		return uuid.Nil(), errors.New("empty session response")
	}
	userID, err := uuid.Parse(response.GetUserId())
	if err != nil || userID == uuid.Nil() {
		return uuid.Nil(), app.ErrUnauthenticated
	}
	return userID, nil
}

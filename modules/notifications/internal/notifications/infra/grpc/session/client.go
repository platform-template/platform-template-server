package session

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	sessionapi "session/api/server/internalapi"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var ErrInvalidSession = errors.New("invalid session")

type Client struct {
	client sessionapi.SessionServiceClient
}

func NewClient(conn grpc.ClientConnInterface) *Client {
	return &Client{client: sessionapi.NewSessionServiceClient(conn)}
}

func (c *Client) Resolve(ctx context.Context, token string) (uuid.UUID, error) {
	response, err := c.client.ResolveSession(ctx, &sessionapi.ResolveSessionRequest{Token: token})
	if err != nil {
		if status.Code(err) == codes.Unauthenticated || status.Code(err) == codes.InvalidArgument {
			return uuid.Nil(), ErrInvalidSession
		}
		return uuid.Nil(), fmt.Errorf("resolve session: %w", err)
	}
	userID, err := uuid.Parse(response.GetUserId())
	if err != nil || userID == uuid.Nil() {
		return uuid.Nil(), errors.New("invalid session user ID")
	}
	return userID, nil
}

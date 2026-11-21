package session

import (
	"context"
	"fmt"
	"uuid"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sessionapi "session/api/server/internalapi"
)

var ErrInvalidSession = fmt.Errorf("invalid session")

type Client struct {
	client sessionapi.SessionServiceClient
}

func NewClient(conn grpc.ClientConnInterface) *Client {
	return &Client{client: sessionapi.NewSessionServiceClient(conn)}
}

func (c *Client) Resolve(
	ctx context.Context,
	token string,
) (uuid.UUID, error) {
	response, err := c.client.ResolveSession(
		ctx,
		&sessionapi.ResolveSessionRequest{Token: token},
	)
	if err != nil {
		if status.Code(err) == codes.Unauthenticated ||
			status.Code(err) == codes.InvalidArgument {
			return uuid.Nil(), ErrInvalidSession
		}
		return uuid.Nil(), fmt.Errorf("resolve session: %w", err)
	}
	userID, err := uuid.Parse(response.GetUserId())
	if err != nil {
		return uuid.Nil(), fmt.Errorf("parse session user ID: %w", err)
	}
	return userID, nil
}

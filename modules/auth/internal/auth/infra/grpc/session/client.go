package session

package session

import (
	"context"
	"fmt"
	"uuid"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sessionapi "session/api/server/internalapi"

	"auth/internal/auth/app"
)

type Client struct {
	client sessionapi.SessionServiceClient
}

var _ app.SessionClient = (*Client)(nil)

func NewClient(conn grpc.ClientConnInterface) *Client {
	return &Client{
		client: sessionapi.NewSessionServiceClient(conn),
	}
}

func (c *Client) Create(ctx context.Context, userID uuid.UUID) (string, error) {
	resp, err := c.client.CreateSession(ctx, &sessionapi.CreateSessionRequest{
		UserId: userID.String(),
	})
	if err != nil {
		return "", fmt.Errorf("session create: %w", err)
	}

	return resp.GetToken(), nil
}

func (c *Client) Resolve(ctx context.Context, token string) (uuid.UUID, error) {
	resp, err := c.client.ResolveSession(ctx, &sessionapi.ResolveSessionRequest{
		Token: token,
	})
	if err != nil {
		if status.Code(err) == codes.Unauthenticated || status.Code(err) == codes.InvalidArgument {
			return uuid.Nil(), fmt.Errorf("session resolve: %w", app.ErrInvalidSession)
		}
		return uuid.Nil(), fmt.Errorf("session resolve: %w", err)
	}

	userID, err := uuid.Parse(resp.GetUserId())
	if err != nil {
		return uuid.Nil(), fmt.Errorf("session resolve: parse user ID: %w", err)
	}

	return userID, nil
}

func (c *Client) RevokeByToken(ctx context.Context, token string) error {
	_, err := c.client.RevokeSession(ctx, &sessionapi.RevokeSessionRequest{
		Token: token,
	})
	if err != nil {
		return fmt.Errorf("session revoke by token: %w", err)
	}

	return nil
}

func (c *Client) RevokeOthersByUserID(ctx context.Context, userID uuid.UUID, token string) error {
	_, err := c.client.RevokeOthersByUserID(ctx, &sessionapi.RevokeOthersByUserIDRequest{
		UserId: userID.String(),
		Token:  token,
	})
	if err != nil {
		return fmt.Errorf("session revoke other sessions: %w", err)
	}

	return nil
}

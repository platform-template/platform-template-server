package identity

import (
	"context"
	"fmt"
	"uuid"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityapi "identity/api/server/internalapi"

	"auth/internal/auth/app"
)

type Client struct {
	client identityapi.IdentityServiceClient
}

var _ app.IdentityClient = (*Client)(nil)

func NewClient(conn grpc.ClientConnInterface) *Client {
	return &Client{
		client: identityapi.NewIdentityServiceClient(conn),
	}
}

func (c *Client) Create(ctx context.Context, firstName, lastName, middleName, login, email string) (uuid.UUID, error) {
	resp, err := c.client.CreateUser(ctx, &identityapi.CreateUserRequest{
		FirstName:  firstName,
		LastName:   lastName,
		MiddleName: middleName,
		Login:      login,
		Email:      email,
	})
	if err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return uuid.Nil(), fmt.Errorf("identity create user: %w", app.ErrLoginAlreadyExists)
		}
		return uuid.Nil(), fmt.Errorf("identity create user: %w", err)
	}

	userID, err := uuid.Parse(resp.GetUserId())
	if err != nil {
		return uuid.Nil(), fmt.Errorf("identity create user: parse user ID: %w", err)
	}

	return userID, nil
}

func (c *Client) FindByLogin(ctx context.Context, login string) (uuid.UUID, error) {
	resp, err := c.client.FindUserByLogin(ctx, &identityapi.FindUserByLoginRequest{
		Login: login,
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return uuid.Nil(), fmt.Errorf("identity find by login: %w", app.ErrUserNotFound)
		}
		return uuid.Nil(), fmt.Errorf("identity find by login: %w", err)
	}

	userID, err := uuid.Parse(resp.GetUserId())
	if err != nil {
		return uuid.Nil(), fmt.Errorf("identity find by login: parse user ID: %w", err)
	}

	return userID, nil
}

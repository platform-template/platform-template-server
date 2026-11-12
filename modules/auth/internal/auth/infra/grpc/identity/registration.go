package identity

import (
	"context"
	"fmt"
	"uuid"

	identityapi "identity/api/server/internalapi"

	"auth/internal/auth/app"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

var _ app.RegistrationIdentityClient = (*Client)(nil)

func (c *Client) Prepare(
	ctx context.Context,
	transactionID uuid.UUID,
	userID uuid.UUID,
	firstName, lastName, middleName, login, email string,
) error {
	_, err := c.client.PrepareRegistration(
		ctx,
		&identityapi.PrepareRegistrationRequest{
			TransactionId: transactionID.String(),
			UserId:        userID.String(),
			FirstName:     firstName,
			LastName:      lastName,
			MiddleName:    optionalString(middleName),
			Login:         login,
			Email:         email,
		},
	)
	if err != nil {
		switch status.Code(err) {
		case codes.AlreadyExists:
			return fmt.Errorf(
				"identity: prepare registration: %w",
				app.ErrLoginAlreadyExists,
			)

		case codes.InvalidArgument:
			return fmt.Errorf(
				"identity: prepare registration: %w",
				app.ErrInvalidUserData,
			)

		default:
			return fmt.Errorf("identity: prepare registration: %w", err)
		}
	}
	return nil
}

func (c *Client) Commit(
	ctx context.Context,
	transactionID uuid.UUID,
) error {
	_, err := c.client.CommitRegistration(
		ctx,
		&identityapi.RegistrationTransactionRequest{
			TransactionId: transactionID.String(),
		},
	)
	if err != nil {
		return fmt.Errorf("identity: commit registration: %w", err)
	}

	return nil
}

func (c *Client) Rollback(
	ctx context.Context,
	transactionID uuid.UUID,
) error {
	_, err := c.client.RollbackRegistration(
		ctx,
		&identityapi.RegistrationTransactionRequest{
			TransactionId: transactionID.String(),
		},
	)
	if err != nil {
		return fmt.Errorf("identity: rollback registration: %w", err)
	}

	return nil
}

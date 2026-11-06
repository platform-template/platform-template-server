package app

import (
	"context"
	"uuid"
)

type IdentityClient interface {
	Create(ctx context.Context, firstName, lastName, middleName, login, email string) (uuid.UUID, error)
}

type SessionClient interface {
	Create(ctx context.Context, userID uuid.UUID) (string, error)
	Resolve(ctx context.Context, token string) (uuid.UUID, error)
	RevokeSession(ctx context.Context, token string) error
	RevokeOthersByUserID(ctx context.Context, userID uuid.UUID, token string) error
}

type RegistrationIdentityClient interface {
	Prepare(
		ctx context.Context,
		transactionID uuid.UUID,
		userID uuid.UUID,
		firstName, lastName, middleName, login, email string,
	) error

	Commit(
		ctx context.Context,
		transactionID uuid.UUID,
	) error

	Rollback(
		ctx context.Context,
		transactionID uuid.UUID,
	) error
}

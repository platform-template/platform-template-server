package app

import (
	"context"
	"uuid"
)

type IdentityClient interface {
	Create(ctx context.Context, firstName, lastName, middleName, login, email string) (uuid.UUID, error)
	FindByLogin(ctx context.Context, login string) (uuid.UUID, error)
}

type SessionClient interface {
	Create(ctx context.Context, userID uuid.UUID) (string, error)
	Resolve(ctx context.Context, token string) (uuid.UUID, error)
	RevokeByToken(ctx context.Context, token string) error
}

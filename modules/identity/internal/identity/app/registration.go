package app

import (
	"context"
	"uuid"
)

type RegistrationParticipant interface {
	Prepare(context.Context, uuid.UUID, uuid.UUID, string, string, *string, string, string) error
	Commit(context.Context, uuid.UUID) error
	Rollback(context.Context, uuid.UUID) error
}

package app

import (
	"context"
	"errors"
	"uuid"
)

type RegistrationState string

const (
	RegistrationPreparing     RegistrationState = "PREPARING"
	RegistrationCommitDecided RegistrationState = "COMMIT_DECIDED"
	RegistrationAbortDecided  RegistrationState = "ABORT_DECIDED"
	RegistrationCommitted     RegistrationState = "COMMITTED"
	RegistrationAborted       RegistrationState = "ABORTED"
)

var (
	ErrRegistrationNotFound      = errors.New("registration transaction not found")
	ErrRegistrationStateConflict = errors.New("registration transaction state conflict")
)

type RegistrationTransaction struct {
	TransactionID uuid.UUID
	UserID        uuid.UUID
	State         RegistrationState
}

type RegistrationJournal interface {
	Begin(ctx context.Context, transactionID, userID uuid.UUID) error

	DecideCommit(ctx context.Context, transactionID uuid.UUID) error
	DecideAbort(ctx context.Context, transactionID uuid.UUID) error

	MarkCommitted(ctx context.Context, transactionID uuid.UUID) error
	MarkAborted(ctx context.Context, transactionID uuid.UUID) error

	Get(ctx context.Context, transactionID uuid.UUID) (RegistrationTransaction, error)
	ListPending(ctx context.Context) ([]RegistrationTransaction, error)
}

package app

import (
	"context"
	"errors"
	"time"
	"uuid"
)

type RegistrationLease struct {
	TransactionID uuid.UUID
	OwnerID       uuid.UUID
	Epoch         uint64
}

var ErrRegistrationLeaseLost = errors.New(
	"registration transaction lease lost",
)

type RegistrationLeaseStore interface {
	Begin(
		ctx context.Context,
		transactionID, userID uuid.UUID,
		ttl time.Duration,
	) (RegistrationLease, error)

	Acquire(
		ctx context.Context,
		transactionID uuid.UUID,
		ttl time.Duration,
	) (RegistrationLease, bool, error)

	Renew(
		ctx context.Context,
		lease RegistrationLease,
		ttl time.Duration,
	) (bool, error)

	Release(
		ctx context.Context,
		lease RegistrationLease,
	) (bool, error)
}

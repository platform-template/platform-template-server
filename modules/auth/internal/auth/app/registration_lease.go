package app

import (
	"context"
	"time"
	"uuid"
)

type RegistrationLease struct {
	TransactionID uuid.UUID
	OwnerID       uuid.UUID
	Epoch         uint64
}

type RegistrationLeaseStore interface {
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

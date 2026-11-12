package registrationjournal

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	"uuid"

	"auth/internal/auth/app"
)

type LeaseStore struct {
	db *sql.DB
}

func NewLeaseStore(db *sql.DB) *LeaseStore {
	return &LeaseStore{db: db}
}

var _ app.RegistrationLeaseStore = (*LeaseStore)(nil)

func leaseMicroseconds(ttl time.Duration) (int64, error) {
	if ttl < time.Microsecond {
		return 0, fmt.Errorf("invalid registration lease TTL: %s", ttl)
	}

	return ttl.Microseconds(), nil
}

func (s *LeaseStore) Acquire(
	ctx context.Context,
	transactionID uuid.UUID,
	ttl time.Duration,
) (app.RegistrationLease, bool, error) {
	microseconds, err := leaseMicroseconds(ttl)
	if err != nil {
		return app.RegistrationLease{}, false, err
	}

	ownerID := uuid.New()

	result, err := s.db.ExecContext(ctx, `
UPDATE registration_transactions
SET lease_owner = ?,
    lease_until = DATE_ADD(
        UTC_TIMESTAMP(6),
        INTERVAL ? MICROSECOND
    ),
    lease_epoch = lease_epoch + 1
WHERE transaction_id = ?
  AND state IN (
      'PREPARING',
      'COMMIT_DECIDED',
      'ABORT_DECIDED'
  )
  AND (
      lease_until IS NULL
      OR lease_until <= UTC_TIMESTAMP(6)
  )
`, ownerID.String(), microseconds, transactionID.String())

	if err != nil {
		return app.RegistrationLease{}, false,
			fmt.Errorf("acquire registration lease: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return app.RegistrationLease{}, false, err
	}
	if affected == 0 {
		return app.RegistrationLease{}, false, nil
	}

	var epoch uint64

	err = s.db.QueryRowContext(ctx, `
SELECT lease_epoch
FROM registration_transactions
WHERE transaction_id = ?
  AND lease_owner = ?
  AND lease_until > UTC_TIMESTAMP(6)
`, transactionID.String(), ownerID.String()).Scan(&epoch)

	if err != nil {
		return app.RegistrationLease{}, false,
			fmt.Errorf("read acquired registration lease: %w", err)
	}

	return app.RegistrationLease{
		TransactionID: transactionID,
		OwnerID:       ownerID,
		Epoch:         epoch,
	}, true, nil
}

func (s *LeaseStore) Renew(
	ctx context.Context,
	lease app.RegistrationLease,
	ttl time.Duration,
) (bool, error) {
	microseconds, err := leaseMicroseconds(ttl)
	if err != nil {
		return false, err
	}

	result, err := s.db.ExecContext(ctx, `
UPDATE registration_transactions
SET lease_until = DATE_ADD(
    UTC_TIMESTAMP(6),
    INTERVAL ? MICROSECOND
)
WHERE transaction_id = ?
  AND lease_owner = ?
  AND lease_epoch = ?
  AND lease_until > UTC_TIMESTAMP(6)
  AND state IN (
      'PREPARING',
      'COMMIT_DECIDED',
      'ABORT_DECIDED'
  )
`,
		microseconds,
		lease.TransactionID.String(),
		lease.OwnerID.String(),
		lease.Epoch,
	)

	if err != nil {
		return false, fmt.Errorf("renew registration lease: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	return affected == 1, nil
}

func (s *LeaseStore) Release(
	ctx context.Context,
	lease app.RegistrationLease,
) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
UPDATE registration_transactions
SET lease_owner = NULL,
    lease_until = NULL
WHERE transaction_id = ?
  AND lease_owner = ?
  AND lease_epoch = ?
`,
		lease.TransactionID.String(),
		lease.OwnerID.String(),
		lease.Epoch,
	)

	if err != nil {
		return false, fmt.Errorf("release registration lease: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}

	return affected == 1, nil
}

package registrationjournal

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"uuid"

	"auth/internal/auth/app"
)

type RegistrationJournal struct {
	db *sql.DB
}

var _ app.RegistrationJournal = (*RegistrationJournal)(nil)

func NewRegistrationJournal(db *sql.DB) *RegistrationJournal {
	return &RegistrationJournal{db: db}
}

func (j *RegistrationJournal) DecideCommit(
	ctx context.Context,
	lease app.RegistrationLease,
) error {
	return transitionOwned(
		ctx, j.db, lease,
		app.RegistrationPreparing,
		app.RegistrationCommitDecided,
	)
}

func (j *RegistrationJournal) DecideAbort(
	ctx context.Context,
	lease app.RegistrationLease,
) error {
	return transitionOwned(
		ctx, j.db, lease,
		app.RegistrationPreparing,
		app.RegistrationAbortDecided,
	)
}

func (j *RegistrationJournal) MarkCommitted(
	ctx context.Context,
	lease app.RegistrationLease,
) error {
	return transitionOwned(
		ctx, j.db, lease,
		app.RegistrationCommitDecided,
		app.RegistrationCommitted,
	)
}

func (j *RegistrationJournal) MarkAborted(
	ctx context.Context,
	lease app.RegistrationLease,
) error {
	return transitionOwned(
		ctx, j.db, lease,
		app.RegistrationAbortDecided,
		app.RegistrationAborted,
	)
}

func (j *RegistrationJournal) transition(
	ctx context.Context,
	transactionID uuid.UUID,
	from, to app.RegistrationState,
) error {
	result, err := j.db.ExecContext(ctx, `
UPDATE registration_transactions
SET state = ?
WHERE transaction_id = ? AND state = ?
`, string(to), transactionID.String(), string(from))
	if err != nil {
		return fmt.Errorf("registration journal: transition: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("registration journal: rows affected: %w", err)
	}

	if affected == 1 {
		return nil
	}

	current, err := j.Get(ctx, transactionID)
	if err != nil {
		return err
	}

	if current.State == to {
		return nil
	}

	if to == app.RegistrationCommitDecided &&
		current.State == app.RegistrationCommitted {
		return nil
	}
	if to == app.RegistrationAbortDecided &&
		current.State == app.RegistrationAborted {
		return nil
	}

	return fmt.Errorf(
		"%w: transaction %s, current=%s, expected=%s, next=%s",
		app.ErrRegistrationStateConflict,
		transactionID,
		current.State,
		from,
		to,
	)
}

func (j *RegistrationJournal) Get(
	ctx context.Context,
	transactionID uuid.UUID,
) (app.RegistrationTransaction, error) {
	var userIDString, state string

	err := j.db.QueryRowContext(ctx, `
SELECT user_id, state
FROM registration_transactions
WHERE transaction_id = ?
`, transactionID.String()).Scan(&userIDString, &state)

	if errors.Is(err, sql.ErrNoRows) {
		return app.RegistrationTransaction{}, app.ErrRegistrationNotFound
	}
	if err != nil {
		return app.RegistrationTransaction{},
			fmt.Errorf("registration journal: get: %w", err)
	}

	userID, err := uuid.Parse(userIDString)
	if err != nil {
		return app.RegistrationTransaction{},
			fmt.Errorf("registration journal: parse user ID: %w", err)
	}

	return app.RegistrationTransaction{
		TransactionID: transactionID,
		UserID:        userID,
		State:         app.RegistrationState(state),
	}, nil
}

func (j *RegistrationJournal) ListPending(
	ctx context.Context,
) ([]app.RegistrationTransaction, error) {
	rows, err := j.db.QueryContext(ctx, `
SELECT transaction_id, user_id, state
FROM registration_transactions
WHERE state IN (?, ?, ?)
ORDER BY created_at, transaction_id
`,
		string(app.RegistrationPreparing),
		string(app.RegistrationCommitDecided),
		string(app.RegistrationAbortDecided),
	)
	if err != nil {
		return nil, fmt.Errorf("registration journal: list pending: %w", err)
	}
	defer rows.Close()

	var transactions []app.RegistrationTransaction

	for rows.Next() {
		var transactionIDString, userIDString, state string

		if err := rows.Scan(
			&transactionIDString,
			&userIDString,
			&state,
		); err != nil {
			return nil, fmt.Errorf("registration journal: scan: %w", err)
		}

		transactionID, err := uuid.Parse(transactionIDString)
		if err != nil {
			return nil, fmt.Errorf("registration journal: parse transaction ID: %w", err)
		}

		userID, err := uuid.Parse(userIDString)
		if err != nil {
			return nil, fmt.Errorf("registration journal: parse user ID: %w", err)
		}

		transactions = append(transactions, app.RegistrationTransaction{
			TransactionID: transactionID,
			UserID:        userID,
			State:         app.RegistrationState(state),
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("registration journal: iterate: %w", err)
	}

	return transactions, nil
}

func transitionOwned(
	ctx context.Context,
	db *sql.DB,
	lease app.RegistrationLease,
	from, to app.RegistrationState,
) error {
	result, err := db.ExecContext(ctx, `
UPDATE registration_transactions
SET state = ?
WHERE transaction_id = ?
  AND state = ?
  AND lease_owner = ?
  AND lease_epoch = ?
  AND lease_until > UTC_TIMESTAMP(6)
`,
		string(to),
		lease.TransactionID.String(),
		string(from),
		lease.OwnerID.String(),
		lease.Epoch,
	)
	if err != nil {
		return fmt.Errorf(
			"transition registration %s from %s to %s: %w",
			lease.TransactionID, from, to, err,
		)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("registration transition rows affected: %w", err)
	}
	if affected != 1 {
		return app.ErrRegistrationStateConflict
	}

	return nil
}

package app

import (
	"context"
	"errors"
	"fmt"
	"uuid"
)

func (c *RegistrationCoordinator) finishAbort(
	ctx context.Context,
	transactionID uuid.UUID,
) error {
	settleCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		registrationSettlementTimeout,
	)
	defer cancel()

	authErr := c.auth.Rollback(settleCtx, transactionID)
	identityErr := c.identity.Rollback(settleCtx, transactionID)

	if err := errors.Join(authErr, identityErr); err != nil {
		return fmt.Errorf(
			"%w: rollback participants: %w",
			ErrRegistrationPending,
			err,
		)
	}

	if err := c.journal.MarkAborted(settleCtx, transactionID); err != nil {
		return fmt.Errorf(
			"%w: mark aborted: %w",
			ErrRegistrationPending,
			err,
		)
	}

	return nil
}

func (c *RegistrationCoordinator) RecoverPending(
	ctx context.Context,
) error {
	pending, err := c.journal.ListPending(ctx)
	if err != nil {
		return fmt.Errorf(
			"registration recovery: list pending: %w",
			err,
		)
	}

	var recoveryErr error

	for _, tx := range pending {
		if err := ctx.Err(); err != nil {
			return errors.Join(recoveryErr, err)
		}

		switch tx.State {
		case RegistrationCommitDecided:
			if err := c.commit(ctx, tx.TransactionID); err != nil {
				recoveryErr = errors.Join(
					recoveryErr,
					fmt.Errorf(
						"registration recovery: commit %s: %w",
						tx.TransactionID,
						err,
					),
				)
			}

		case RegistrationAbortDecided:
			if err := c.finishAbort(ctx, tx.TransactionID); err != nil {
				recoveryErr = errors.Join(
					recoveryErr,
					fmt.Errorf(
						"registration recovery: abort %s: %w",
						tx.TransactionID,
						err,
					),
				)
			}

		case RegistrationPreparing:
			recoveryErr = errors.Join(
				recoveryErr,
				fmt.Errorf(
					"%w: transaction %s still preparing",
					ErrRegistrationPending,
					tx.TransactionID,
				),
			)

		default:
			recoveryErr = errors.Join(
				recoveryErr,
				fmt.Errorf(
					"registration recovery: unexpected state %s for %s",
					tx.State,
					tx.TransactionID,
				),
			)
		}
	}

	return recoveryErr
}

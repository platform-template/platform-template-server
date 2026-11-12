package app

import (
	"context"
	"errors"
	"fmt"
)

func (c *RegistrationCoordinator) finishAbort(
	ctx context.Context,
	lease RegistrationLease,
) error {
	settleCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		registrationSettlementTimeout,
	)
	defer cancel()

	authErr := c.auth.Rollback(settleCtx, lease.TransactionID)
	identityErr := c.identity.Rollback(settleCtx, lease.TransactionID)

	if err := errors.Join(authErr, identityErr); err != nil {
		return fmt.Errorf(
			"%w: rollback participants: %w",
			ErrRegistrationPending,
			err,
		)
	}

	if err := c.journal.MarkAborted(settleCtx, lease); err != nil {
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
			"registration recovery: list pending: %w", err,
		)
	}

	var recoveryErr error

	for _, pendingTx := range pending {
		if err := ctx.Err(); err != nil {
			return errors.Join(recoveryErr, err)
		}

		lease, acquired, err := c.leases.Acquire(
			ctx,
			pendingTx.TransactionID,
			registrationLeaseTTL,
		)
		if err != nil {
			recoveryErr = errors.Join(
				recoveryErr,
				fmt.Errorf(
					"registration recovery: acquire %s: %w",
					pendingTx.TransactionID,
					err,
				),
			)
			continue
		}

		// Транзакция принадлежит активному координатору.
		if !acquired {
			continue
		}

		err = c.withLease(ctx, lease, func(ctx context.Context) error {
			// После Acquire перечитываем состояние.
			// Снимок ListPending мог устареть.
			tx, err := c.journal.Get(ctx, lease.TransactionID)
			if err != nil {
				return fmt.Errorf("read acquired transaction: %w", err)
			}

			switch tx.State {
			case RegistrationPreparing:
				if err := c.journal.DecideAbort(ctx, lease); err != nil {
					return fmt.Errorf(
						"%w: persist ABORT decision: %w",
						ErrRegistrationPending,
						err,
					)
				}

				return c.finishAbort(ctx, lease)

			case RegistrationCommitDecided:
				return c.commit(ctx, lease)

			case RegistrationAbortDecided:
				return c.finishAbort(ctx, lease)

			case RegistrationCommitted, RegistrationAborted:
				return nil

			default:
				return fmt.Errorf(
					"unexpected registration state %q",
					tx.State,
				)
			}
		})

		if err != nil {
			recoveryErr = errors.Join(
				recoveryErr,
				fmt.Errorf(
					"registration recovery: transaction %s: %w",
					pendingTx.TransactionID,
					err,
				),
			)
		}
	}

	return recoveryErr
}

package app

import (
	"context"
	"errors"
	"fmt"
)

func (c *RegistrationCoordinator) RecoverCommits(
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

		case RegistrationPreparing, RegistrationAbortDecided:
			recoveryErr = errors.Join(
				recoveryErr,
				fmt.Errorf(
					"%w: transaction %s in state %s",
					ErrRegistrationPending,
					tx.TransactionID,
					tx.State,
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

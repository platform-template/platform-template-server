package app

import (
	"context"
	"errors"
	"fmt"
	"time"
	"uuid"

	"auth/internal/auth/domain"
)

const registrationSettlementTimeout = 15 * time.Second

var ErrRegistrationPending = errors.New(
	"registration transaction requires reconciliation",
)

type RegistrationAuthParticipant interface {
	Prepare(
		ctx context.Context,
		transactionID, userID uuid.UUID,
		login, password string,
	) error

	Commit(ctx context.Context, transactionID uuid.UUID) error
	Rollback(ctx context.Context, transactionID uuid.UUID) error
}

type RegistrationCoordinator struct {
	journal  RegistrationJournal
	leases   RegistrationLeaseStore
	auth     RegistrationAuthParticipant
	identity RegistrationIdentityClient
}

func NewRegistrationCoordinator(
	journal RegistrationJournal,
	leases RegistrationLeaseStore,
	auth RegistrationAuthParticipant,
	identity RegistrationIdentityClient,
) *RegistrationCoordinator {
	return &RegistrationCoordinator{
		journal:  journal,
		leases:   leases,
		auth:     auth,
		identity: identity,
	}
}

func (c *RegistrationCoordinator) Register(
	ctx context.Context,
	input RegisterInput,
) (uuid.UUID, error) {
	if err := domain.ValidatePassword(input.Password); err != nil {
		return uuid.Nil(), fmt.Errorf(
			"register: validate password: %w", err,
		)
	}

	transactionID := uuid.New()
	userID := uuid.New()

	lease, err := c.leases.Begin(
		ctx,
		transactionID,
		userID,
		registrationLeaseTTL,
	)
	if err != nil {
		return uuid.Nil(), fmt.Errorf(
			"register: begin transaction: %w", err,
		)
	}

	err = c.withLease(ctx, lease, func(ctx context.Context) error {
		if err := c.auth.Prepare(
			ctx,
			transactionID,
			userID,
			input.Login,
			input.Password,
		); err != nil {
			return c.abort(
				ctx, lease,
				fmt.Errorf("register: prepare auth: %w", err),
			)
		}

		if err := c.identity.Prepare(
			ctx,
			transactionID,
			userID,
			input.FirstName,
			input.LastName,
			input.MiddleName,
			input.Login,
			input.Email,
		); err != nil {
			return c.abort(
				ctx, lease,
				fmt.Errorf("register: prepare identity: %w", err),
			)
		}

		if err := c.journal.DecideCommit(ctx, lease); err != nil {
			return fmt.Errorf(
				"%w: persist COMMIT decision: %w",
				ErrRegistrationPending,
				err,
			)
		}

		return c.commit(ctx, lease)
	})

	return userID, err
}

func (c *RegistrationCoordinator) commit(
	ctx context.Context,
	lease RegistrationLease,
) error {
	settleCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		registrationSettlementTimeout,
	)
	defer cancel()

	authErr := c.auth.Commit(settleCtx, lease.TransactionID)
	identityErr := c.identity.Commit(settleCtx, lease.TransactionID)

	if err := errors.Join(authErr, identityErr); err != nil {
		return fmt.Errorf(
			"%w: commit participants: %w",
			ErrRegistrationPending,
			err,
		)
	}

	if err := c.journal.MarkCommitted(settleCtx, lease); err != nil {
		return fmt.Errorf(
			"%w: mark committed: %w",
			ErrRegistrationPending,
			err,
		)
	}

	return nil
}

func (c *RegistrationCoordinator) abort(
	ctx context.Context,
	lease RegistrationLease,
	cause error,
) error {
	settleCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		registrationSettlementTimeout,
	)
	defer cancel()

	if err := c.journal.DecideAbort(settleCtx, lease); err != nil {
		return errors.Join(
			cause,
			fmt.Errorf(
				"%w: persist ABORT decision: %w",
				ErrRegistrationPending,
				err,
			),
		)
	}

	authErr := c.auth.Rollback(settleCtx, lease.TransactionID)
	identityErr := c.identity.Rollback(settleCtx, lease.TransactionID)

	if err := errors.Join(authErr, identityErr); err != nil {
		return errors.Join(
			cause,
			fmt.Errorf(
				"%w: rollback participants: %w",
				ErrRegistrationPending,
				err,
			),
		)
	}

	if err := c.journal.MarkAborted(settleCtx, lease); err != nil {
		return errors.Join(
			cause,
			fmt.Errorf(
				"%w: mark aborted: %w",
				ErrRegistrationPending,
				err,
			),
		)
	}

	return cause
}

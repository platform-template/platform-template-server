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
	auth     RegistrationAuthParticipant
	identity RegistrationIdentityClient
}

func NewRegistrationCoordinator(
	journal RegistrationJournal,
	auth RegistrationAuthParticipant,
	identity RegistrationIdentityClient,
) *RegistrationCoordinator {
	return &RegistrationCoordinator{
		journal:  journal,
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

	if err := c.journal.Begin(ctx, transactionID, userID); err != nil {
		return uuid.Nil(), fmt.Errorf(
			"register: begin transaction: %w", err,
		)
	}

	if err := c.auth.Prepare(
		ctx,
		transactionID,
		userID,
		input.Login,
		input.Password,
	); err != nil {
		return userID, c.abort(
			ctx,
			transactionID,
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
		return userID, c.abort(
			ctx,
			transactionID,
			fmt.Errorf("register: prepare identity: %w", err),
		)
	}

	if err := c.journal.DecideCommit(ctx, transactionID); err != nil {
		return userID, fmt.Errorf(
			"%w: persist COMMIT decision: %w",
			ErrRegistrationPending,
			err,
		)
	}

	return userID, c.commit(ctx, transactionID)
}

func (c *RegistrationCoordinator) commit(
	ctx context.Context,
	transactionID uuid.UUID,
) error {
	settleCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		registrationSettlementTimeout,
	)
	defer cancel()

	authErr := c.auth.Commit(settleCtx, transactionID)
	identityErr := c.identity.Commit(settleCtx, transactionID)

	if err := errors.Join(authErr, identityErr); err != nil {
		return fmt.Errorf(
			"%w: commit participants: %w",
			ErrRegistrationPending,
			err,
		)
	}

	if err := c.journal.MarkCommitted(settleCtx, transactionID); err != nil {
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
	transactionID uuid.UUID,
	cause error,
) error {
	settleCtx, cancel := context.WithTimeout(
		context.WithoutCancel(ctx),
		registrationSettlementTimeout,
	)
	defer cancel()

	if err := c.journal.DecideAbort(settleCtx, transactionID); err != nil {
		return errors.Join(
			cause,
			fmt.Errorf(
				"%w: persist ABORT decision: %w",
				ErrRegistrationPending,
				err,
			),
		)
	}

	authErr := c.auth.Rollback(settleCtx, transactionID)
	identityErr := c.identity.Rollback(settleCtx, transactionID)

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

	if err := c.journal.MarkAborted(settleCtx, transactionID); err != nil {
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

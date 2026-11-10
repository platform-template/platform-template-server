package app

import (
	"context"
	"fmt"
	"uuid"

	"github.com/distributed-programming-2026/lib/transaction/xa"

	"auth/internal/auth/domain"
	authmysql "auth/internal/auth/infra/mysql"
	"auth/internal/auth/infra/mysql/repository"
)

const registrationBranch = "auth"

type RegistrationParticipant struct {
	resource xa.Resource
	hasher   domain.PasswordHasher
}

func NewRegistrationParticipant(
	resource xa.Resource,
	hasher domain.PasswordHasher,
) *RegistrationParticipant {
	return &RegistrationParticipant{
		resource: resource,
		hasher:   hasher,
	}
}

func (p *RegistrationParticipant) Prepare(
	ctx context.Context,
	transactionID uuid.UUID,
	userID uuid.UUID,
	login string,
	password string,
) error {
	xid := xa.XID{
		TransactionID: transactionID,
		Branch:        registrationBranch,
	}

	err := p.resource.Prepare(
		ctx,
		xid,
		func(ctx context.Context, executor xa.Executor) error {
			client := authmysql.NewXAClient(executor)
			repo := repository.NewAuthRepository(ctx, client)

			svc := domain.NewAuthService(repo, p.hasher)

			if err := svc.Create(userID, login, password); err != nil {
				return err
			}

			return xa.RecordCommitReceipt(ctx, executor, xid)
		},
	)
	if err != nil {
		return fmt.Errorf("auth: prepare registration: %w", err)
	}

	return nil
}

func (p *RegistrationParticipant) Commit(
	ctx context.Context,
	transactionID uuid.UUID,
) error {
	xid := xa.XID{
		TransactionID: transactionID,
		Branch:        registrationBranch,
	}

	if err := p.resource.Commit(ctx, xid); err != nil {
		return fmt.Errorf("auth: commit registration: %w", err)
	}

	return nil
}

func (p *RegistrationParticipant) Rollback(
	ctx context.Context,
	transactionID uuid.UUID,
) error {
	xid := xa.XID{
		TransactionID: transactionID,
		Branch:        registrationBranch,
	}

	if err := p.resource.Rollback(ctx, xid); err != nil {
		return fmt.Errorf("auth: rollback registration: %w", err)
	}

	return nil
}

package app

import (
	"context"
	"fmt"
	"uuid"

	"github.com/distributed-programming-2026/lib/transaction/xa"

	"identity/internal/identity/domain"
	"identity/internal/identity/infra/mysql"
	"identity/internal/identity/infra/mysql/repository"
)

const registrationBranch = "identity"

type RegistrationService struct {
	resource xa.Resource
}

func NewRegistrationService(resource xa.Resource) *RegistrationService {
	return &RegistrationService{resource: resource}
}

func (s *RegistrationService) Prepare(
	ctx context.Context,
	transactionID uuid.UUID,
	userID uuid.UUID,
	firstName string,
	lastName string,
	middleName *string,
	loginSnapshot string,
	email string,
) error {
	xid := xa.XID{
		TransactionID: transactionID,
		Branch:        registrationBranch,
	}

	err := s.resource.Prepare(ctx, xid, func(ctx context.Context, executor xa.Executor) error {
		client := mysql.NewXAClient(executor)
		repo := repository.NewIdentityRepository(ctx, client)
		svc := domain.NewIdentityService(repo)

		return svc.CreateWithID(userID, firstName, lastName, middleName, loginSnapshot, email)
	})
	if err != nil {
		return fmt.Errorf("identity: prepare registration: %w", err)
	}

	return nil
}

func (s *RegistrationService) Commit(ctx context.Context, transactionID uuid.UUID) error {
	xid := xa.XID{
		TransactionID: transactionID,
		Branch:        registrationBranch,
	}

	if err := s.resource.Commit(ctx, xid); err != nil {
		return fmt.Errorf("identity: commit registration: %w", err)
	}

	return nil
}

func (s *RegistrationService) Rollback(ctx context.Context, transactionID uuid.UUID) error {
	xid := xa.XID{
		TransactionID: transactionID,
		Branch:        registrationBranch,
	}

	if err := s.resource.Rollback(ctx, xid); err != nil {
		return fmt.Errorf("identity: rollback registration: %w", err)
	}

	return nil
}

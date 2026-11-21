package mysql

import (
	"context"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"

	"authz/internal/authz/domain"
	"authz/internal/authz/infra/mysql/repository"
)

func NewRepositoryProvider(client mysql.ClientContext) *RepositoryProvider {
	return &RepositoryProvider{client: client}
}

type RepositoryProvider struct {
	client mysql.ClientContext
}

func (p *RepositoryProvider) ModuleGrant(ctx context.Context) domain.ModuleGrantRepository {
	return repository.NewModuleGrantRepository(ctx, p.client)
}

func (p *RepositoryProvider) SystemRole(ctx context.Context) domain.SystemRoleRepository {
	return repository.NewSystemRoleRepository(ctx, p.client)
}

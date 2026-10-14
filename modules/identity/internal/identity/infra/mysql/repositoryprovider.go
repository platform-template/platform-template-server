package mysql

import (
	"context"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"

	"identity/internal/identity/domain"
	"identity/internal/identity/infra/mysql/repository"
)

func NewRepositoryProvider(client mysql.ClientContext) *RepositoryProvider {
	return &RepositoryProvider{
		client: client,
	}
}

type RepositoryProvider struct {
	client mysql.ClientContext
}

func (p *RepositoryProvider) Identity(ctx context.Context) domain.IdentityRepository {
	return repository.NewIdentityRepository(ctx, p.client)
}

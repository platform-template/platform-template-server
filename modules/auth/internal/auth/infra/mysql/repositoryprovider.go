package mysql

import (
	"context"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"

	"auth/internal/auth/domain"
	"auth/internal/auth/infra/mysql/repository"
)

func NewRepositoryProvider(client mysql.ClientContext) *RepositoryProvider {
	return &RepositoryProvider{
		client: client,
	}
}

type RepositoryProvider struct {
	client mysql.ClientContext
}

func (p *RepositoryProvider) Echo(ctx context.Context) domain.EchoRepository {
	return repository.NewEchoRepository(ctx, p.client)
}

package mysql

import (
	"context"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"

	"shopping/internal/shopping/domain"
	"shopping/internal/shopping/infra/mysql/repository"
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

func (p *RepositoryProvider) List(ctx context.Context) domain.ListRepository {
	return repository.NewListRepository(ctx, p.client)
}

func (p *RepositoryProvider) Demand(ctx context.Context) domain.DemandRepository {
	return repository.NewDemandRepository(ctx, p.client)
}

func (p *RepositoryProvider) Purchase(ctx context.Context) domain.PurchaseRepository {
	return repository.NewPurchaseRepository(ctx, p.client)
}

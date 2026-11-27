package mysql

import (
	"context"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"

	"mealplanner/internal/mealplanner/domain"
	"mealplanner/internal/mealplanner/infra/mysql/repository"
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

func (p *RepositoryProvider) Plan(ctx context.Context) domain.PlanRepository {
	return repository.NewPlanRepository(ctx, p.client)
}

func (p *RepositoryProvider) Item(ctx context.Context) domain.ItemRepository {
	return repository.NewItemRepository(ctx, p.client)
}

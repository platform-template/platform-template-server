package mysql

import (
	"context"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"

	"recipes/internal/recipes/domain"
	"recipes/internal/recipes/infra/mysql/repository"
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

func (p *RepositoryProvider) Recipe(ctx context.Context) domain.RecipeRepository {
	return repository.NewRecipeRepository(ctx, p.client)
}

func (p *RepositoryProvider) RecipeOwnership(ctx context.Context) domain.RecipeOwnership {
	return repository.NewOwnershipRepository(ctx, p.client)
}

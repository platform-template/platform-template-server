package mysql

import (
	"context"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"

	"inventory/internal/inventory/domain"
	"inventory/internal/inventory/infra/mysql/repository"
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

func (p *RepositoryProvider) Product(ctx context.Context) domain.ProductRepository {
	return repository.NewProductRepository(ctx, p.client)
}

func (p *RepositoryProvider) Inventory(ctx context.Context) domain.InventoryRepository {
	return repository.NewInventoryRepository(ctx, p.client)
}

func (p *RepositoryProvider) Stock(ctx context.Context) domain.StockRepository {
	return repository.NewStockRepository(ctx, p.client)
}

func (p *RepositoryProvider) Reservation(ctx context.Context) domain.ReservationRepository {
	return repository.NewReservationRepository(ctx, p.client)
}

func (p *RepositoryProvider) InventoryOwnership(ctx context.Context) domain.InventoryOwnership {
	return repository.NewOwnershipRepository(ctx, p.client)
}

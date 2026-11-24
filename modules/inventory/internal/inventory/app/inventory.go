package app

import (
	"context"
	"fmt"
	"uuid"

	"inventory/internal/inventory/domain"
	"inventory/internal/inventory/infra/mysql"
)

func (s *Service) CreateProduct(
	ctx context.Context,
	name string,
	unit domain.Unit,
) (*domain.Product, error) {
	product, err := domain.NewProduct(uuid.New(), name, unit)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	err = s.unit.ExecuteWithRepositoryProvider(
		ctx,
		func(provider *mysql.RepositoryProvider) error {
			return provider.Product(ctx).Create(product)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("create product: %w", err)
	}
	return &product, nil
}

func (s *Service) ListProducts(ctx context.Context, filter domain.ProductFilter) ([]domain.Product, error) {
	if filter.Limit < 0 || filter.Limit > 100 || filter.Offset < 0 {
		return nil, domain.ErrInvalidProductFilter
	}
	if filter.Limit == 0 {
		filter.Limit = 20
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var products []domain.Product
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		var err error
		products, err = provider.Product(ctx).List(filter)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	if products == nil {
		products = make([]domain.Product, 0)
	}
	return products, nil
}

func (s *Service) GetProduct(ctx context.Context, id uuid.UUID) (*domain.Product, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var product *domain.Product
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		var err error
		product, err = provider.Product(ctx).GetByID(id)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	if product == nil {
		return nil, domain.ErrProductNotFound
	}
	return product, nil
}

func (s *Service) GetStock(ctx context.Context, id uuid.UUID) (*domain.Stock, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var stock *domain.Stock
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		var err error
		stock, err = provider.Stock(ctx).GetByProductID(id)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("get stock: %w", err)
	}
	if stock == nil {
		return nil, domain.ErrProductNotFound
	}
	return stock, nil
}

func (s *Service) Consume(ctx context.Context, id uuid.UUID) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		return provider.Reservation(ctx).Consume(id)
	})
	if err != nil {
		return fmt.Errorf("consume: %w", err)
	}
	return nil
}

func (s *Service) AddStock(ctx context.Context, id uuid.UUID, quantity int64) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		return provider.Stock(ctx).Add(id, quantity)
	})
	if err != nil {
		return fmt.Errorf("add stock: %w", err)
	}
	return nil
}

func (s *Service) Reserve(ctx context.Context, id uuid.UUID, items []domain.ReservationItem) error {
	reservation, err := domain.NewReservation(id, items)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	err = s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		return provider.Reservation(ctx).Reserve(reservation)
	})
	if err != nil {
		return fmt.Errorf("reserve: %w", err)
	}
	return nil
}

func (s *Service) Release(ctx context.Context, id uuid.UUID) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		return provider.Reservation(ctx).Release(id)
	})
	if err != nil {
		return fmt.Errorf("release: %w", err)
	}
	return nil
}

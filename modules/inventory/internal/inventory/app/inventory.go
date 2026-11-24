package app

import (
	"context"
	"fmt"
	"uuid"

	"inventory/internal/inventory/domain"
	"inventory/internal/inventory/infra/mysql"
)

func (s *Service) CreateProduct(ctx context.Context, product domain.Product) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		return provider.Product(ctx).Create(product)
	})
	if err != nil {
		return fmt.Errorf("create product: %w", err)
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

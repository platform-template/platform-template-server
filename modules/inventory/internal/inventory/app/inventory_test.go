package app

import (
	"context"
	"errors"
	"testing"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	"github.com/stretchr/testify/require"

	"inventory/internal/inventory/domain"
	"inventory/internal/inventory/infra/mysql"
)

type failingUnit struct {
	uow.UnitOfWorkWithRepositoryProvider[*mysql.RepositoryProvider]
	err error
}

func (u failingUnit) ExecuteWithRepositoryProvider(_ context.Context, _ func(*mysql.RepositoryProvider) error) error {
	return u.err
}

func TestInventoryUnitErrors(t *testing.T) {
	expected := errors.New("transaction failed")
	svc := NewService(failingUnit{err: expected}, nil)
	_, err := svc.CreateProduct(t.Context(), "milk", domain.UnitMillilitres)
	require.ErrorIs(t, err, expected)
	_, err = svc.CreateProduct(t.Context(), " ", domain.UnitMillilitres)
	require.ErrorIs(t, err, domain.ErrInvalidProduct)
	_, err = svc.ListProducts(t.Context(), domain.ProductFilter{})
	require.ErrorIs(t, err, expected)
	_, err = svc.ListProducts(t.Context(), domain.ProductFilter{Limit: 101})
	require.ErrorIs(t, err, domain.ErrInvalidProductFilter)
	_, err = svc.GetProduct(t.Context(), uuid.New())
	require.ErrorIs(t, err, expected)
	_, err = svc.GetStock(t.Context(), uuid.New())
	require.ErrorIs(t, err, expected)
	require.ErrorIs(t, svc.AddStock(t.Context(), uuid.New(), 1), expected)
	require.ErrorIs(t, svc.Consume(t.Context(), uuid.New()), expected)
}

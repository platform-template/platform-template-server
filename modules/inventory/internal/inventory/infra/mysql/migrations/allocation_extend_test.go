//go:build integration

package migrations_test

import (
	"context"
	"errors"
	"testing"
	"uuid"

	"inventory/internal/inventory/app"
	"inventory/internal/inventory/domain"
	inventorymysql "inventory/internal/inventory/infra/mysql"

	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	"github.com/stretchr/testify/require"
)

func testAdditionalAllocation(
	t *testing.T,
	service *app.Service,
	unit uow.UnitOfWorkWithRepositoryProvider[*inventorymysql.RepositoryProvider],
) {
	t.Helper()

	tests := []struct {
		name    string
		consume bool
	}{
		{
			name:    "cancel releases all reservation batches",
			consume: false,
		},
		{
			name:    "consume deducts all reservation batches",
			consume: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()

			inventory, err := service.CreateInventory(ctx, "Холодос Величайшего")
			require.NoError(t, err)

			product, err := service.CreateProduct(
				ctx,
				"Рис для соло-керри "+uuid.New().String(),
				domain.Unit("g"),
			)
			require.NoError(t, err)

			inventoryID := inventory.ID()
			productID := product.ID()
			operationID := uuid.New()

			inventories := []uuid.UUID{inventoryID}
			requirements := []domain.ReservationItem{
				{ProductID: productID, Quantity: 1000},
			}

			execute := func(fn func(domain.AllocationRepository) error) error {
				callCtx, cancel := context.WithCancel(ctx)
				defer cancel()

				return unit.ExecuteWithRepositoryProvider(callCtx, func(p *inventorymysql.RepositoryProvider) error {
					return fn(p.Allocation(callCtx))
				})
			}

			assertStock := func(quantity, reserved int64) {
				t.Helper()

				stock, err := service.GetStock(ctx, inventoryID, productID)
				require.NoError(t, err)
				require.NotNil(t, stock)
				require.Equal(t, quantity, stock.Quantity)
				require.Equal(t, reserved, stock.ReservedQuantity)
			}

			require.NoError(t, service.AddStock(ctx, inventoryID, productID, 600))

			var result domain.AllocationResult
			var changed bool

			require.NoError(t, execute(func(repo domain.AllocationRepository) error {
				var err error
				result, changed, err = repo.Allocate(operationID, inventories, requirements)
				return err
			}))

			require.True(t, changed)
			require.EqualValues(t, 1, result.ResultVersion)
			require.Len(t, result.Lines, 1)
			require.EqualValues(t, 600, result.Lines[0].Quantity)
			require.EqualValues(t, 400, result.Shortages[0].Quantity)

			originalReservationID := result.Lines[0].ReservationID

			assertStock(600, 600)

			require.NoError(t, service.AddStock(ctx, inventoryID, productID, 250))

			extend := func() error {
				return execute(func(repo domain.AllocationRepository) error {
					var err error
					result, changed, err = repo.Extend(operationID, inventories, requirements)
					return err
				})
			}

			failure := errors.New("rollback additional allocation")

			err = execute(func(repo domain.AllocationRepository) error {
				_, _, err := repo.Extend(operationID, inventories, requirements)
				if err != nil {
					return err
				}

				return failure
			})

			require.ErrorIs(t, err, failure)

			assertStock(850, 600)

			require.NoError(t, extend())
			require.True(t, changed)
			require.EqualValues(t, 2, result.ResultVersion)
			require.Len(t, result.Lines, 2)

			require.Equal(t, originalReservationID, result.Lines[0].ReservationID)
			require.EqualValues(t, 600, result.Lines[0].Quantity)
			require.EqualValues(t, 250, result.Lines[1].Quantity)
			require.EqualValues(t, 2, result.Lines[1].BatchVersion)
			require.NotEqual(t, originalReservationID, result.Lines[1].ReservationID)

			require.Len(t, result.Shortages, 1)
			require.EqualValues(t, 150, result.Shortages[0].Quantity)

			assertStock(850, 850)

			previous := result

			require.NoError(t, extend())
			require.False(t, changed)
			require.Equal(t, previous, result)

			require.NoError(t, service.AddStock(ctx, inventoryID, productID, 200))

			require.NoError(t, extend())
			require.True(t, changed)
			require.EqualValues(t, 3, result.ResultVersion)
			require.Len(t, result.Lines, 3)
			require.Empty(t, result.Shortages)

			require.Equal(t, originalReservationID, result.Lines[0].ReservationID)
			require.EqualValues(t, 150, result.Lines[2].Quantity)
			require.EqualValues(t, 3, result.Lines[2].BatchVersion)

			assertStock(1050, 1000)

			require.NoError(t, extend())
			require.False(t, changed)
			require.EqualValues(t, 3, result.ResultVersion)

			require.NoError(t, execute(func(repo domain.AllocationRepository) error {
				var err error

				if tt.consume {
					changed, err = repo.Consume(operationID)
				} else {
					changed, err = repo.Cancel(operationID)
				}

				return err
			}))

			require.True(t, changed)

			if tt.consume {
				assertStock(50, 0)
			} else {
				assertStock(1050, 0)
			}

			err = extend()

			if tt.consume {
				require.ErrorIs(t, err, domain.ErrReservationConsumed)
			} else {
				require.ErrorIs(t, err, domain.ErrAllocationCancelled)
			}
		})
	}
}

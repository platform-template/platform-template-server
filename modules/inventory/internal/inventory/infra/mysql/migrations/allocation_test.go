//go:build integration

package migrations_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"uuid"

	"inventory/internal/inventory/app"
	"inventory/internal/inventory/domain"
	inventorymysql "inventory/internal/inventory/infra/mysql"

	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	"github.com/stretchr/testify/require"
)

func testPartialAllocation(
	t *testing.T,
	service *app.Service,
	unit uow.UnitOfWorkWithRepositoryProvider[*inventorymysql.RepositoryProvider],
) {
	t.Helper()
	ctx := t.Context()
	first, err := service.CreateInventory(ctx, "First allocation inventory")
	require.NoError(t, err)
	second, err := service.CreateInventory(ctx, "Second allocation inventory")
	require.NoError(t, err)
	product, err := service.CreateProduct(ctx, "Allocation test product", domain.Unit("g"))
	require.NoError(t, err)
	require.NoError(t, service.AddStock(ctx, first.ID(), product.ID(), 3))
	require.NoError(t, service.AddStock(ctx, second.ID(), product.ID(), 2))
	operation := uuid.New()
	inventories := []uuid.UUID{first.ID(), second.ID()}
	required := []domain.ReservationItem{{ProductID: product.ID(), Quantity: 8}}
	var result domain.AllocationResult
	var created bool
	execute := func(id uuid.UUID, items []domain.ReservationItem) error {
		callCtx, cancel := context.WithCancel(ctx)
		defer cancel()
		return unit.ExecuteWithRepositoryProvider(callCtx, func(p *inventorymysql.RepositoryProvider) error {
			var err error
			result, created, err = p.Allocation(callCtx).Allocate(id, inventories, items)
			return err
		})
	}
	require.NoError(t, execute(operation, required))
	require.True(t, created)
	require.Len(t, result.Lines, 2)
	require.Equal(t, []domain.ReservationItem{{ProductID: product.ID(), Quantity: 3}}, result.Shortages)
	for _, line := range result.Lines {
		require.NotEqual(t, uuid.Nil(), line.ReservationID)
	}
	original := result
	require.NoError(t, execute(operation, required))
	require.False(t, created)
	require.Equal(t, original, result)
	require.ErrorIs(
		t,
		execute(operation, []domain.ReservationItem{{ProductID: product.ID(), Quantity: 7}}),
		domain.ErrAllocationConflict,
	)
	stockA, err := service.GetStock(ctx, first.ID(), product.ID())
	require.NoError(t, err)
	require.EqualValues(t, 3, stockA.ReservedQuantity)
	stockB, err := service.GetStock(ctx, second.ID(), product.ID())
	require.NoError(t, err)
	require.EqualValues(t, 2, stockB.ReservedQuantity)
	failure := errors.New("rollback")
	rollbackID := uuid.New()
	callCtx, done := context.WithCancel(ctx)
	err = unit.ExecuteWithRepositoryProvider(callCtx, func(p *inventorymysql.RepositoryProvider) error {
		_, _, err := p.Allocation(callCtx).Allocate(rollbackID, inventories, required)
		if err != nil {
			return err
		}
		return failure
	})
	done()
	require.ErrorIs(t, err, failure)
	require.NoError(t, execute(rollbackID, required))
	require.True(t, created)
	require.Empty(t, result.Lines)
	require.Equal(t, required, result.Shortages)

	concurrentProduct, err := service.CreateProduct(ctx, "Concurrent allocation product", domain.Unit("g"))
	require.NoError(t, err)
	require.NoError(t, service.AddStock(ctx, first.ID(), concurrentProduct.ID(), 5))
	start := make(chan struct{})
	outcomes := make(chan domain.AllocationResult, 2)
	failures := make(chan error, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			callCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var allocation domain.AllocationResult
			err := unit.ExecuteWithRepositoryProvider(callCtx, func(p *inventorymysql.RepositoryProvider) error {
				var err error
				allocation, _, err = p.Allocation(callCtx).
					Allocate(uuid.New(), []uuid.UUID{first.ID()}, []domain.ReservationItem{{ProductID: concurrentProduct.ID(), Quantity: 4}})
				return err
			})
			if err != nil {
				failures <- err
				return
			}
			outcomes <- allocation
		}()
	}
	close(start)
	workers.Wait()
	close(failures)
	close(outcomes)
	for err := range failures {
		require.NoError(t, err)
	}
	totalReserved, totalShortage := int64(0), int64(0)
	for allocation := range outcomes {
		for _, line := range allocation.Lines {
			totalReserved += line.Quantity
		}
		for _, shortage := range allocation.Shortages {
			totalShortage += shortage.Quantity
		}
	}
	require.EqualValues(t, 5, totalReserved)
	require.EqualValues(t, 3, totalShortage)
}

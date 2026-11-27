package domain

import (
	"testing"
	"uuid"

	"github.com/stretchr/testify/require"
)

func TestDistributePartialAcrossInventories(t *testing.T) {
	a := uuid.MustParse("00000000-0000-4000-8000-000000000001")
	b := uuid.MustParse("00000000-0000-4000-8000-000000000002")
	milk, rice := uuid.New(), uuid.New()
	requirements := []ReservationItem{{ProductID: milk, Quantity: 7}, {ProductID: rice, Quantity: 4}}
	stocks := []Stock{
		{InventoryID: a, ProductID: milk, Quantity: 5, ReservedQuantity: 2},
		{InventoryID: b, ProductID: milk, Quantity: 2},
		{InventoryID: b, ProductID: rice, Quantity: 6, ReservedQuantity: 1},
	}
	result, err := DistributePartial([]uuid.UUID{b, a}, requirements, stocks)
	require.NoError(t, err)
	require.Contains(t, result.Lines, AllocationLine{InventoryID: a, ProductID: milk, Quantity: 3})
	require.Contains(t, result.Lines, AllocationLine{InventoryID: b, ProductID: milk, Quantity: 2})
	require.Contains(t, result.Lines, AllocationLine{InventoryID: b, ProductID: rice, Quantity: 4})
	require.Equal(t, []ReservationItem{{ProductID: milk, Quantity: 2}}, result.Shortages)
	again, err := DistributePartial([]uuid.UUID{a, b}, []ReservationItem{requirements[1], requirements[0]}, stocks)
	require.NoError(t, err)
	require.Equal(t, result, again)
}
func TestDistributePartialRejectsInvalidInput(t *testing.T) {
	inventoryID, productID := uuid.New(), uuid.New()
	req := []ReservationItem{{ProductID: productID, Quantity: 1}}
	_, err := DistributePartial([]uuid.UUID{inventoryID, inventoryID}, req, nil)
	require.ErrorIs(t, err, ErrInvalidAllocation)
	_, err = DistributePartial([]uuid.UUID{inventoryID}, append(req, req...), nil)
	require.ErrorIs(t, err, ErrInvalidAllocation)
	_, err = DistributePartial(
		[]uuid.UUID{inventoryID},
		req,
		[]Stock{{InventoryID: inventoryID, ProductID: productID, Quantity: 1, ReservedQuantity: 2}},
	)
	require.ErrorIs(t, err, ErrInvalidAllocation)
	result, err := DistributePartial([]uuid.UUID{inventoryID}, req, nil)
	require.NoError(t, err)
	require.Empty(t, result.Lines)
	require.Equal(t, req, result.Shortages)
}

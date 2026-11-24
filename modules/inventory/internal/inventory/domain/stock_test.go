package domain

import (
	"testing"
	"uuid"

	"github.com/stretchr/testify/require"
)

func TestStock(t *testing.T) {
	inventoryID := uuid.New()
	productID := uuid.New()
	_, err := NewStock(inventoryID, productID, 1, 2)
	require.ErrorIs(t, err, ErrInvalidStock)
	_, err = NewStock(inventoryID, productID, -1, 0)
	require.ErrorIs(t, err, ErrInvalidStock)
	stock, err := NewStock(inventoryID, productID, 10, 0)
	require.NoError(t, err)
	_, err = stock.Reserve(0)
	require.ErrorIs(t, err, ErrInvalidReservation)
	_, err = stock.Reserve(11)
	require.ErrorIs(t, err, ErrInsufficientStock)
	stock, err = stock.Reserve(7)
	require.NoError(t, err)
	require.EqualValues(t, 3, stock.Available())
	consumed, err := stock.Consume(7)
	require.NoError(t, err)
	require.EqualValues(t, 3, consumed.Quantity)
	require.Zero(t, consumed.ReservedQuantity)
	require.Equal(t, stock.Available(), consumed.Available())
	_, err = stock.Consume(8)
	require.ErrorIs(t, err, ErrInvalidStock)
	stock, err = stock.Release(7)
	require.NoError(t, err)
	require.EqualValues(t, 10, stock.Available())
	_, err = stock.Release(7)
	require.ErrorIs(t, err, ErrInvalidStock)
}

func TestNilInventoryID(t *testing.T) {
	inventoryID := uuid.Nil()
	productID := uuid.New()
	_, err := NewStock(inventoryID, productID, 2, 1)
	require.ErrorIs(t, err, ErrInvalidStock)
}

func TestRemove(t *testing.T) {
	inventoryID := uuid.New()
	productID := uuid.New()
	stock, err := NewStock(inventoryID, productID, 5, 1)
	require.NoError(t, err)

	stock, err = stock.Remove(2)
	require.NoError(t, err)
	require.Equal(t, int64(3), stock.Quantity)
}

func TestRemoveAll(t *testing.T) {
	inventoryID := uuid.New()
	productID := uuid.New()
	stock, err := NewStock(inventoryID, productID, 5, 0)
	require.NoError(t, err)

	stock, err = stock.Remove(5)
	require.NoError(t, err)
	require.Zero(t, stock.Quantity)
}

func TestRemoveMoreThanAvailable(t *testing.T) {
	inventoryID := uuid.New()
	productID := uuid.New()
	stock, err := NewStock(inventoryID, productID, 5, 1)
	require.NoError(t, err)

	_, err = stock.Remove(5)
	require.ErrorIs(t, err, ErrInsufficientStock)
}

func TestRemoveInvalidQuantity(t *testing.T) {
	inventoryID := uuid.New()
	productID := uuid.New()
	stock, err := NewStock(inventoryID, productID, 5, 1)
	require.NoError(t, err)

	_, err = stock.Remove(-2)
	require.ErrorIs(t, err, ErrInvalidQuantityToRemove)
}

func TestImmutabilityOfInventoryID(t *testing.T) {
	inventoryID := uuid.New()
	productID := uuid.New()
	stock, err := NewStock(inventoryID, productID, 5, 1)
	require.NoError(t, err)

	stock, err = stock.Reserve(1)
	require.NoError(t, err)

	stock, err = stock.Release(2)
	require.NoError(t, err)

	require.Equal(t, inventoryID, stock.InventoryID)
}

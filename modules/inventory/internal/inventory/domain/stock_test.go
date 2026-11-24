package domain

import (
	"testing"
	"uuid"

	"github.com/stretchr/testify/require"
)

func TestStock(t *testing.T) {
	id := uuid.New()
	_, err := NewStock(id, 1, 2)
	require.ErrorIs(t, err, ErrInvalidStock)
	_, err = NewStock(id, -1, 0)
	require.ErrorIs(t, err, ErrInvalidStock)
	stock, err := NewStock(id, 10, 0)
	require.NoError(t, err)
	_, err = stock.Reserve(0)
	require.ErrorIs(t, err, ErrInvalidReservation)
	_, err = stock.Reserve(11)
	require.ErrorIs(t, err, ErrInsufficientStock)
	stock, err = stock.Reserve(7)
	require.NoError(t, err)
	require.EqualValues(t, 3, stock.Available())
	stock, err = stock.Release(7)
	require.NoError(t, err)
	require.EqualValues(t, 10, stock.Available())
	_, err = stock.Release(7)
	require.ErrorIs(t, err, ErrInvalidStock)
}

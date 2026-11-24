package domain

import (
	"testing"
	"uuid"

	"github.com/stretchr/testify/require"
)

func TestProduct(t *testing.T) {
	id := uuid.New()
	for _, unit := range []Unit{UnitGrams, UnitMillilitres, UnitPieces} {
		product, err := NewProduct(id, "salt", unit)
		require.NoError(t, err)
		require.Equal(t, unit, product.Unit)
	}
	_, err := NewProduct(id, "salt", "kg")
	require.ErrorIs(t, err, ErrInvalidProduct)
	_, err = NewProduct(id, " ", UnitGrams)
	require.ErrorIs(t, err, ErrInvalidProduct)
}

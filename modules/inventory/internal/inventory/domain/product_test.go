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
		require.Equal(t, unit, product.Unit())
	}
	_, err := NewProduct(id, "salt", "kg")
	require.ErrorIs(t, err, ErrInvalidProduct)
	_, err = NewProduct(id, " ", UnitGrams)
	require.ErrorIs(t, err, ErrInvalidProduct)
}

func TestProductRename(t *testing.T) {
	product, err := NewProduct(uuid.New(), "греча", UnitGrams)
	require.NoError(t, err)

	err = product.Rename("рис")
	require.NoError(t, err)
	require.Equal(t, "рис", product.Name())
	require.Equal(t, UnitGrams, product.Unit())

	err = product.Rename("  ")
	require.ErrorIs(t, err, ErrInvalidProduct)
	require.Equal(t, "рис", product.Name())
}

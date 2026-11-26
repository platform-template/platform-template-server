package domain

import (
	"testing"
	"uuid"

	"github.com/stretchr/testify/require"
)

func TestInventoryRename(t *testing.T) {
	id := uuid.New()
	inventory, err := NewInventory(id, "Холодильник")
	require.NoError(t, err)

	err = inventory.Rename("Морозилка")
	require.NoError(t, err)
	require.Equal(t, id, inventory.ID())
	require.Equal(t, "Морозилка", inventory.Name())

	err = inventory.Rename("  ")
	require.ErrorIs(t, err, ErrInvalidInventoryName)
	require.Equal(t, "Морозилка", inventory.Name())
}

func TestNewInventoryRejectsInvalidValues(t *testing.T) {
	_, err := NewInventory(uuid.Nil(), "Холодильник")
	require.ErrorIs(t, err, ErrInvalidInventory)

	_, err = NewInventory(uuid.New(), "\t")
	require.ErrorIs(t, err, ErrInvalidInventory)
}

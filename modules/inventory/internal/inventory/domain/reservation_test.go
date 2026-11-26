package domain

import (
	"testing"
	"uuid"

	"github.com/stretchr/testify/require"
)

func TestReservation(t *testing.T) {
	id, productID, inventoryID := uuid.New(), uuid.New(), uuid.New()
	item := ReservationItem{
		ProductID: productID,
		Quantity:  2,
	}
	_, err := NewReservation(id, inventoryID, []ReservationItem{item, item})
	require.ErrorIs(t, err, ErrInvalidReservation)
	_, err = NewReservation(id, inventoryID, []ReservationItem{
		{
			ProductID: productID,
			Quantity:  0,
		},
	})
	require.ErrorIs(t, err, ErrInvalidReservation)
	reservation, err := NewReservation(id, inventoryID, []ReservationItem{item})
	require.NoError(t, err)
	items := reservation.Items()
	items[0].Quantity = 99
	require.EqualValues(t, 2, reservation.Items()[0].Quantity)
	changed, err := reservation.Cancel()
	require.NoError(t, err)
	require.True(t, changed)
	changed, err = reservation.Cancel()
	require.NoError(t, err)
	require.False(t, changed)
	_, err = reservation.Consume()
	require.ErrorIs(t, err, ErrReservationCancelled)
	require.Equal(t, ReservationCancelled, reservation.Status())
	marker, err := RestoreReservation(id, inventoryID, ReservationCancelled, nil)
	require.NoError(t, err)
	changed, err = marker.Cancel()
	require.NoError(t, err)
	require.False(t, changed)
	consumed, err := NewReservation(id, inventoryID, []ReservationItem{item})
	require.NoError(t, err)
	changed, err = consumed.Consume()
	require.NoError(t, err)
	require.True(t, changed)
	changed, err = consumed.Consume()
	require.NoError(t, err)
	require.False(t, changed)
	_, err = consumed.Cancel()
	require.ErrorIs(t, err, ErrReservationConsumed)
}

func TestMatchComparesInventoryID(t *testing.T) {
	id, productID, inventoryID := uuid.New(), uuid.New(), uuid.New()
	inventoryID2 := uuid.New()
	item1 := ReservationItem{
		ProductID: productID,
		Quantity:  2,
	}
	reservation1, err := NewReservation(id, inventoryID, []ReservationItem{item1})
	require.NoError(t, err)
	reservation2, err := NewReservation(id, inventoryID2, []ReservationItem{
		{
			ProductID: productID,
			Quantity:  2,
		},
	})
	require.NoError(t, err)
	reservation3, err := NewReservation(id, inventoryID, []ReservationItem{
		{
			ProductID: productID,
			Quantity:  2,
		},
	})
	require.NoError(t, err)

	require.False(t, reservation1.Matches(reservation2))
	require.True(t, reservation1.Matches(reservation3))
}

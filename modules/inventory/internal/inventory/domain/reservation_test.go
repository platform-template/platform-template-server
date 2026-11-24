package domain

import (
	"testing"
	"uuid"

	"github.com/stretchr/testify/require"
)

func TestReservation(t *testing.T) {
	id, productID := uuid.New(), uuid.New()
	item := ReservationItem{
		ProductID: productID,
		Quantity:  2,
	}
	_, err := NewReservation(id, []ReservationItem{item, item})
	require.ErrorIs(t, err, ErrInvalidReservation)
	_, err = NewReservation(id, []ReservationItem{
		{
			ProductID: productID,
			Quantity:  0,
		},
	})
	require.ErrorIs(t, err, ErrInvalidReservation)
	reservation, err := NewReservation(id, []ReservationItem{item})
	require.NoError(t, err)
	items := reservation.Items()
	items[0].Quantity = 99
	require.EqualValues(t, 2, reservation.Items()[0].Quantity)
	require.True(t, reservation.Cancel())
	require.False(t, reservation.Cancel())
	require.Equal(t, ReservationCancelled, reservation.Status())
	marker, err := RestoreReservation(id, ReservationCancelled, nil)
	require.NoError(t, err)
	require.False(t, marker.Cancel())
}

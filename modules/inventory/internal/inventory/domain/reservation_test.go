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
	changed, err := reservation.Cancel()
	require.NoError(t, err)
	require.True(t, changed)
	changed, err = reservation.Cancel()
	require.NoError(t, err)
	require.False(t, changed)
	_, err = reservation.Consume()
	require.ErrorIs(t, err, ErrReservationCancelled)
	require.Equal(t, ReservationCancelled, reservation.Status())
	marker, err := RestoreReservation(id, ReservationCancelled, nil)
	require.NoError(t, err)
	changed, err = marker.Cancel()
	require.NoError(t, err)
	require.False(t, changed)
	consumed, err := NewReservation(id, []ReservationItem{item})
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

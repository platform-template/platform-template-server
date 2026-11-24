package domain

import (
	"errors"
	"sort"
	"uuid"
)

var (
	ErrInvalidReservation   = errors.New("invalid reservation")
	ErrReservationConflict  = errors.New("reservation conflict")
	ErrReservationCancelled = errors.New("reservation cancelled")
)

type ReservationItem struct {
	ProductID uuid.UUID
	Quantity  int64
}

type ReservationStatus string

const (
	ReservationActive    ReservationStatus = "active"
	ReservationCancelled ReservationStatus = "cancelled"
)

type Reservation struct {
	id     uuid.UUID
	status ReservationStatus
	items  []ReservationItem
}

func NewReservation(id uuid.UUID, items []ReservationItem) (*Reservation, error) {
	return RestoreReservation(id, ReservationActive, items)
}

func RestoreReservation(
	id uuid.UUID,
	status ReservationStatus,
	items []ReservationItem,
) (*Reservation, error) {
	if id == uuid.Nil() ||
		(status != ReservationActive && status != ReservationCancelled) ||
		(status == ReservationActive && len(items) == 0) {
		return nil, ErrInvalidReservation
	}
	copyItems := append([]ReservationItem(nil), items...)
	sort.Slice(
		copyItems,
		func(i, j int) bool {
			return copyItems[i].ProductID.String() < copyItems[j].ProductID.String()
		},
	)
	for index, item := range copyItems {
		if item.ProductID == uuid.Nil() || item.Quantity <= 0 ||
			(index > 0 && copyItems[index-1].ProductID == item.ProductID) {
			return nil, ErrInvalidReservation
		}
	}
	return &Reservation{
		id:     id,
		status: status,
		items:  copyItems,
	}, nil
}

func (reservation *Reservation) ID() uuid.UUID { return reservation.id }

func (reservation *Reservation) Status() ReservationStatus { return reservation.status }

func (reservation *Reservation) Items() []ReservationItem {
	return append([]ReservationItem(nil), reservation.items...)
}

func (reservation *Reservation) Cancel() bool {
	if reservation.status == ReservationCancelled {
		return false
	}
	reservation.status = ReservationCancelled
	return true
}

func (reservation *Reservation) Matches(other *Reservation) bool {
	if len(reservation.items) != len(other.items) {
		return false
	}
	for index := range reservation.items {
		if reservation.items[index] != other.items[index] {
			return false
		}
	}
	return true
}

type ReservationRepository interface {
	GetByID(uuid.UUID) (*Reservation, error)
	Reserve(*Reservation) error
	Release(uuid.UUID) error
}

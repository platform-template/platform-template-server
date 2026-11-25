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
	ErrReservationConsumed  = errors.New("reservation consumed")
	ErrReservationNotFound  = errors.New("reservation not found")
)

type ReservationItem struct {
	ProductID uuid.UUID
	Quantity  int64
}

type ReservationStatus string

const (
	ReservationActive    ReservationStatus = "active"
	ReservationCancelled ReservationStatus = "cancelled"
	ReservationConsumed  ReservationStatus = "consumed"
)

type Reservation struct {
	id          uuid.UUID
	inventoryID uuid.UUID
	status      ReservationStatus
	items       []ReservationItem
}

func NewReservation(id, inventoryID uuid.UUID, items []ReservationItem) (*Reservation, error) {
	return RestoreReservation(id, inventoryID, ReservationActive, items)
}

func RestoreReservation(
	id uuid.UUID,
	inventoryID uuid.UUID,
	status ReservationStatus,
	items []ReservationItem,
) (*Reservation, error) {
	if id == uuid.Nil() ||
		inventoryID == uuid.Nil() ||
		(status != ReservationActive && status != ReservationCancelled && status != ReservationConsumed) ||
		(status != ReservationCancelled && len(items) == 0) {
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
		id:          id,
		inventoryID: inventoryID,
		status:      status,
		items:       copyItems,
	}, nil
}

func (reservation *Reservation) ID() uuid.UUID { return reservation.id }

func (reservation *Reservation) InventoryID() uuid.UUID { return reservation.inventoryID }

func (reservation *Reservation) Status() ReservationStatus { return reservation.status }

func (reservation *Reservation) Items() []ReservationItem {
	return append([]ReservationItem(nil), reservation.items...)
}

func (reservation *Reservation) Cancel() (bool, error) {
	switch reservation.status {
	case ReservationCancelled:
		return false, nil
	case ReservationConsumed:
		return false, ErrReservationConsumed
	}
	reservation.status = ReservationCancelled
	return true, nil
}

func (reservation *Reservation) Consume() (bool, error) {
	switch reservation.status {
	case ReservationConsumed:
		return false, nil
	case ReservationCancelled:
		return false, ErrReservationCancelled
	}
	reservation.status = ReservationConsumed
	return true, nil
}

func (reservation *Reservation) Matches(other *Reservation) bool {
	if len(reservation.items) != len(other.items) ||
		reservation.inventoryID != other.inventoryID {
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
	GetByID(inventoryID, id uuid.UUID) (*Reservation, error)
	Reserve(*Reservation) error
	Release(inventoryID, id uuid.UUID) error
	Consume(inventoryID, id uuid.UUID) error
}

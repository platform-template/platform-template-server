package domain

import (
	"errors"
	"math"
	"uuid"
)

var (
	ErrInvalidStock      = errors.New("invalid stock")
	ErrInsufficientStock = errors.New("insufficient stock")
)

type Stock struct {
	ProductID        uuid.UUID
	Quantity         int64
	ReservedQuantity int64
}

func NewStock(productID uuid.UUID, quantity, reserved int64) (Stock, error) {
	if productID == uuid.Nil() || quantity < 0 || reserved < 0 || reserved > quantity {
		return Stock{}, ErrInvalidStock
	}
	return Stock{
		ProductID:        productID,
		Quantity:         quantity,
		ReservedQuantity: reserved,
	}, nil
}

func (stock Stock) Available() int64 {
	return stock.Quantity - stock.ReservedQuantity
}

func (stock Stock) Add(quantity int64) (Stock, error) {
	if quantity <= 0 || stock.Quantity > math.MaxInt64-quantity {
		return Stock{}, ErrInvalidStock
	}
	stock.Quantity += quantity
	return stock, nil
}

func (stock Stock) Reserve(quantity int64) (Stock, error) {
	if quantity <= 0 {
		return Stock{}, ErrInvalidReservation
	}
	if quantity > stock.Available() {
		return Stock{}, ErrInsufficientStock
	}
	stock.ReservedQuantity += quantity
	return stock, nil
}

func (stock Stock) Release(quantity int64) (Stock, error) {
	if quantity <= 0 || quantity > stock.ReservedQuantity {
		return Stock{}, ErrInvalidStock
	}
	stock.ReservedQuantity -= quantity
	return stock, nil
}

func (stock Stock) Consume(quantity int64) (Stock, error) {
	if quantity <= 0 || quantity > stock.ReservedQuantity {
		return Stock{}, ErrInvalidStock
	}
	stock.Quantity -= quantity
	stock.ReservedQuantity -= quantity
	return stock, nil
}

type StockRepository interface {
	GetByProductID(uuid.UUID) (*Stock, error)
	GetByProductIDs([]uuid.UUID) ([]Stock, error)
	Add(uuid.UUID, int64) error
}

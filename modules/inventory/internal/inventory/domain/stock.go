package domain

import (
	"errors"
	"math"
	"uuid"
)

var (
	ErrInvalidStock            = errors.New("invalid stock")
	ErrStockExists             = errors.New("stock already exists")
	ErrInsufficientStock       = errors.New("insufficient stock")
	ErrInvalidQuantityToRemove = errors.New("invalid quantity to remove")
)

type Stock struct {
	InventoryID      uuid.UUID
	ProductID        uuid.UUID
	Quantity         int64
	ReservedQuantity int64
}

func NewStock(
	inventoryID uuid.UUID,
	productID uuid.UUID,
	quantity, reserved int64,
) (Stock, error) {
	if inventoryID == uuid.Nil() ||
		productID == uuid.Nil() ||
		quantity < 0 ||
		reserved < 0 ||
		reserved > quantity {
		return Stock{}, ErrInvalidStock
	}
	return Stock{
		InventoryID:      inventoryID,
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

func (stock Stock) Remove(quantity int64) (Stock, error) {
	if quantity <= 0 {
		return Stock{}, ErrInvalidQuantityToRemove
	}
	if stock.Available() < quantity {
		return Stock{}, ErrInsufficientStock
	}

	stock.Quantity -= quantity

	return stock, nil
}

type StockRepository interface {
	Create(stock Stock) error
	GetByProductID(inventoryID, productID uuid.UUID) (*Stock, error)
	GetByProductIDs(inventoryID uuid.UUID, productID []uuid.UUID) ([]Stock, error)
	List(inventoryID uuid.UUID) ([]Stock, error)
	Add(inventoryID, productID uuid.UUID, quantity int64) error
	Replenish(operationID, inventoryID, productID uuid.UUID, quantity int64) (bool, error)
	Remove(inventoryID, productID uuid.UUID, quantity int64) error
	Delete(inventoryID, productID uuid.UUID) error
}

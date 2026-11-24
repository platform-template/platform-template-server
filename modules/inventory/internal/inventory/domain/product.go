package domain

import (
	"errors"
	"strings"
	"uuid"
)

var (
	ErrInvalidProduct  = errors.New("invalid product")
	ErrProductNotFound = errors.New("product not found")
	ErrProductExists   = errors.New("product already exists")
)

type Unit string

const (
	UnitGrams       Unit = "g"
	UnitMillilitres Unit = "ml"
	UnitPieces      Unit = "pcs"
)

type Product struct {
	ID   uuid.UUID
	Name string
	Unit Unit
}

func NewProduct(id uuid.UUID, name string, unit Unit) (Product, error) {
	if id == uuid.Nil() || strings.TrimSpace(name) == "" ||
		(unit != UnitGrams && unit != UnitMillilitres && unit != UnitPieces) {
		return Product{}, ErrInvalidProduct
	}
	return Product{
		ID:   id,
		Name: name,
		Unit: unit,
	}, nil
}

type ProductRepository interface {
	Create(Product) error
	GetByID(uuid.UUID) (*Product, error)
	List() ([]Product, error)
}

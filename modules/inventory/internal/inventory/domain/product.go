package domain

import (
	"errors"
	"strings"
	"uuid"
)

var (
	ErrInvalidProduct       = errors.New("invalid product")
	ErrProductNotFound      = errors.New("product not found")
	ErrProductExists        = errors.New("product already exists")
	ErrInvalidProductFilter = errors.New("invalid product filter")
	ErrProductInUse         = errors.New("product is in use")
)

type Unit string

const (
	UnitGrams       Unit = "g"
	UnitMillilitres Unit = "ml"
	UnitPieces      Unit = "pcs"
)

type Product struct {
	id   uuid.UUID
	name string
	unit Unit
}

func NewProduct(id uuid.UUID, name string, unit Unit) (Product, error) {
	if id == uuid.Nil() || strings.TrimSpace(name) == "" ||
		(unit != UnitGrams && unit != UnitMillilitres && unit != UnitPieces) {
		return Product{}, ErrInvalidProduct
	}
	return Product{
		id:   id,
		name: name,
		unit: unit,
	}, nil
}

func (p *Product) ID() uuid.UUID { return p.id }
func (p *Product) Name() string  { return p.name }
func (p *Product) Unit() Unit    { return p.unit }

func (p *Product) Rename(name string) error {
	if strings.TrimSpace(name) == "" {
		return ErrInvalidProduct
	}
	p.name = name

	return nil
}

type ProductRepository interface {
	Create(*Product) error
	Update(*Product) error
	GetByID(uuid.UUID) (*Product, error)
	List(ProductFilter) ([]*Product, error)
}

type ProductFilter struct {
	Search string
	Limit  int
	Offset int
}

package domain

import (
	"errors"
	"strings"
	"uuid"
)

var (
	ErrInvalidInventory     = errors.New("invalid inventory")
	ErrInvalidInventoryName = errors.New("invalid inventory name")
	ErrInventoryNotFound    = errors.New("inventory not found")
	ErrInventoryExists      = errors.New("inventory already exists")
)

type Inventory struct {
	id   uuid.UUID
	name string
}

func (i Inventory) ID() uuid.UUID {
	return i.id
}

func (i Inventory) Name() string {
	return i.name
}

func (i *Inventory) Rename(name string) error {
	if strings.TrimSpace(name) == "" {
		return ErrInvalidInventoryName
	}
	i.name = name

	return nil
}

func NewInventory(id uuid.UUID, name string) (*Inventory, error) {
	if id == uuid.Nil() || strings.TrimSpace(name) == "" {
		return nil, ErrInvalidInventory
	}

	return &Inventory{
		id:   id,
		name: name,
	}, nil
}

type InventoryRepository interface {
	Create(*Inventory) error
	Update(*Inventory) error
	GetByID(uuid.UUID) (*Inventory, error)
	List() ([]*Inventory, error)
	Delete(uuid.UUID) error
}

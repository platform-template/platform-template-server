package domain

import (
	"errors"
	"strings"
	"uuid"
)

var ErrInvalidList = errors.New("invalid shopping list")

type List struct {
	id, owner uuid.UUID
	name      string
}

func NewList(id, owner uuid.UUID, name string) (*List, error) {
	if id == uuid.Nil() || owner == uuid.Nil() || strings.TrimSpace(name) == "" {
		return nil, ErrInvalidList
	}
	return &List{id: id, owner: owner, name: name}, nil
}
func (l *List) ID() uuid.UUID      { return l.id }
func (l *List) OwnerID() uuid.UUID { return l.owner }
func (l *List) Name() string       { return l.name }
func (l *List) Rename(name string) error {
	if strings.TrimSpace(name) == "" {
		return ErrInvalidList
	}
	l.name = name
	return nil
}

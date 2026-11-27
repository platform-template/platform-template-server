package domain

import (
	"errors"
	"strings"
	"uuid"
)

var ErrInvalidPlan = errors.New("invalid meal plan")

type Plan struct {
	id      uuid.UUID
	ownerID uuid.UUID
	name    string
}

func NewPlan(id, ownerID uuid.UUID, name string) (*Plan, error) {
	if id == uuid.Nil() || ownerID == uuid.Nil() || strings.TrimSpace(name) == "" {
		return nil, ErrInvalidPlan
	}
	return &Plan{id: id, ownerID: ownerID, name: name}, nil
}

func (p *Plan) ID() uuid.UUID      { return p.id }
func (p *Plan) OwnerID() uuid.UUID { return p.ownerID }
func (p *Plan) Name() string       { return p.name }
func (p *Plan) Rename(name string) error {
	if strings.TrimSpace(name) == "" {
		return ErrInvalidPlan
	}
	p.name = name
	return nil
}

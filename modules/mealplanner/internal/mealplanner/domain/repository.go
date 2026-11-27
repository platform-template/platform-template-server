package domain

import (
	"errors"
	"time"
	"uuid"
)

var ErrConcurrentUpdate = errors.New("meal item was modified concurrently")

type PlanRepository interface {
	Create(*Plan) error
	Get(ownerID, planID uuid.UUID) (*Plan, error)
	List(ownerID uuid.UUID) ([]*Plan, error)
	Update(*Plan) error
}

type ItemRepository interface {
	Create(*Item) error
	Get(planID, itemID uuid.UUID) (*Item, uint64, error)
	List(planID uuid.UUID, from, to time.Time) ([]*Item, error)
	Update(*Item, uint64) error
}

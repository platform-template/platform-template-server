package domain

import (
	"errors"
	"math"
	"uuid"
)

var (
	ErrInvalidDemand   = errors.New("invalid shopping demand")
	ErrAutomaticDemand = errors.New("automatic demand cannot be manually modified")
	ErrInvalidPurchase = errors.New("invalid purchase")
	ErrDemandConflict  = errors.New("shopping demand conflict")
)

type Source struct {
	MealPlanID uuid.UUID
	MealItemID uuid.UUID
	Revision   uuid.UUID
}

type DemandData struct {
	ID        uuid.UUID
	ListID    uuid.UUID
	ProductID uuid.UUID
	Required  int64
	Purchased int64
	Cancelled bool
	Source    *Source
}

type Demand struct{ data DemandData }

func NewDemand(data DemandData) (*Demand, error) {
	if data.ID == uuid.Nil() || data.ListID == uuid.Nil() || data.ProductID == uuid.Nil() || data.Required <= 0 ||
		data.Purchased != 0 ||
		data.Cancelled {
		return nil, ErrInvalidDemand
	}
	if data.Source != nil {
		source := *data.Source
		if source.MealPlanID == uuid.Nil() || source.MealItemID == uuid.Nil() || source.Revision == uuid.Nil() {
			return nil, ErrInvalidDemand
		}
		data.Source = &source
	}
	return &Demand{data: data}, nil
}
func RestoreDemand(data DemandData) (*Demand, error) {
	if data.Purchased < 0 {
		return nil, ErrInvalidDemand
	}
	purchased, cancelled := data.Purchased, data.Cancelled
	data.Purchased, data.Cancelled = 0, false
	demand, err := NewDemand(data)
	if err != nil {
		return nil, err
	}
	demand.data.Purchased, demand.data.Cancelled = purchased, cancelled
	return demand, nil
}

func (d *Demand) Data() DemandData {
	result := d.data
	if result.Source != nil {
		source := *result.Source
		result.Source = &source
	}
	return result
}
func (d *Demand) Remaining() int64 {
	if d.data.Cancelled || d.data.Purchased >= d.data.Required {
		return 0
	}
	return d.data.Required - d.data.Purchased
}
func (d *Demand) ChangeManualQuantity(quantity int64) error {
	if d.data.Source != nil {
		return ErrAutomaticDemand
	}
	if d.data.Cancelled || quantity <= 0 || quantity < d.data.Purchased {
		return ErrInvalidDemand
	}
	d.data.Required = quantity
	return nil
}
func (d *Demand) CancelSource(source Source) bool {
	if d.data.Source == nil || *d.data.Source != source || d.data.Cancelled {
		return false
	}
	d.data.Cancelled = true
	return true
}
func (d *Demand) RemoveManual() error {
	if d.data.Source != nil {
		return ErrAutomaticDemand
	}
	d.data.Cancelled = true
	return nil
}

// ApplyPurchase must run in the same transaction as marking the purchase confirmed.
func (d *Demand) ApplyPurchase(purchase *Purchase) (bool, error) {
	if purchase.data.DemandID != d.data.ID || purchase.data.ProductID != d.data.ProductID {
		return false, ErrInvalidPurchase
	}
	if purchase.data.Confirmed {
		return false, nil
	}
	if purchase.data.Quantity > math.MaxInt64-d.data.Purchased {
		return false, ErrInvalidPurchase
	}
	d.data.Purchased += purchase.data.Quantity
	purchase.data.Confirmed = true
	return true, nil
}

type DemandRepository interface {
	CreateManual(*Demand) error
	List(listID uuid.UUID) ([]*Demand, error)
	Get(listID, demandID uuid.UUID) (*Demand, error)
	Lock(listID, demandID uuid.UUID) (*Demand, error)
	Update(*Demand) error
	LockSource(listID uuid.UUID, source Source) (bool, error)
	CancelSource(listID uuid.UUID, source Source) (bool, error)
	RegisterAllocation(revision uuid.UUID, hash [32]byte) (bool, error)
	CreateAutomatic(*Demand) (bool, error)
	ListBySource(listID uuid.UUID, source Source) ([]*Demand, error)
}

type PurchaseRepository interface {
	Create(listID uuid.UUID, purchase *Purchase) (bool, error)
}

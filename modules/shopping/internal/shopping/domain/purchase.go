package domain

import "uuid"

type PurchaseData struct {
	ID          uuid.UUID
	DemandID    uuid.UUID
	ProductID   uuid.UUID
	InventoryID uuid.UUID
	Quantity    int64
	Confirmed   bool
}

type Purchase struct{ data PurchaseData }

func NewPurchase(data PurchaseData) (*Purchase, error) {
	if data.ID == uuid.Nil() || data.DemandID == uuid.Nil() || data.ProductID == uuid.Nil() ||
		data.InventoryID == uuid.Nil() ||
		data.Quantity <= 0 ||
		data.Confirmed {
		return nil, ErrInvalidPurchase
	}
	return &Purchase{data: data}, nil
}
func (p *Purchase) Data() PurchaseData { return p.data }

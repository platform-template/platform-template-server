package domain

import (
	"errors"
	"sort"
	"uuid"
)

var (
	ErrInvalidAllocation         = errors.New("invalid allocation")
	ErrAllocationConflict        = errors.New("allocation request conflict")
	ErrAllocationCancelled       = errors.New("allocation cancelled")
	ErrAllocationIdentityMissing = errors.New("allocation identity missing")
)

type AllocationLine struct {
	ReservationID uuid.UUID
	InventoryID   uuid.UUID
	ProductID     uuid.UUID
	Quantity      int64
}

type AllocationResult struct {
	Lines     []AllocationLine
	Shortages []ReservationItem
}

// DistributePartial deterministically allocates available stock in inventory order.
// The caller must hold transactional locks on all supplied stock rows.
func DistributePartial(
	inventoryIDs []uuid.UUID,
	requirements []ReservationItem,
	stocks []Stock,
) (AllocationResult, error) {
	if len(inventoryIDs) == 0 || len(requirements) == 0 {
		return AllocationResult{}, ErrInvalidAllocation
	}
	inventories := append([]uuid.UUID(nil), inventoryIDs...)
	sort.Slice(inventories, func(i, j int) bool { return inventories[i].String() < inventories[j].String() })
	allowed := make(map[uuid.UUID]bool, len(inventories))
	for _, id := range inventories {
		if id == uuid.Nil() || allowed[id] {
			return AllocationResult{}, ErrInvalidAllocation
		}
		allowed[id] = true
	}
	requested := append([]ReservationItem(nil), requirements...)
	sort.Slice(
		requested,
		func(i, j int) bool { return requested[i].ProductID.String() < requested[j].ProductID.String() },
	)
	required := make(map[uuid.UUID]bool, len(requested))
	for _, item := range requested {
		if item.ProductID == uuid.Nil() || item.Quantity <= 0 || required[item.ProductID] {
			return AllocationResult{}, ErrInvalidAllocation
		}
		required[item.ProductID] = true
	}
	type stockKey struct {
		inventory uuid.UUID
		product   uuid.UUID
	}
	available := make(map[stockKey]int64, len(stocks))
	for _, stock := range stocks {
		key := stockKey{stock.InventoryID, stock.ProductID}
		if !allowed[key.inventory] || !required[key.product] {
			return AllocationResult{}, ErrInvalidAllocation
		}
		if _, exists := available[key]; exists {
			return AllocationResult{}, ErrInvalidAllocation
		}
		if _, err := NewStock(stock.InventoryID, stock.ProductID, stock.Quantity, stock.ReservedQuantity); err != nil {
			return AllocationResult{}, ErrInvalidAllocation
		}
		available[key] = stock.Available()
	}
	result := AllocationResult{Lines: make([]AllocationLine, 0), Shortages: make([]ReservationItem, 0)}
	for _, item := range requested {
		remaining := item.Quantity
		for _, inventoryID := range inventories {
			quantity := min(remaining, available[stockKey{inventoryID, item.ProductID}])
			if quantity > 0 {
				result.Lines = append(
					result.Lines,
					AllocationLine{InventoryID: inventoryID, ProductID: item.ProductID, Quantity: quantity},
				)
				remaining -= quantity
			}
			if remaining == 0 {
				break
			}
		}
		if remaining > 0 {
			result.Shortages = append(result.Shortages, ReservationItem{ProductID: item.ProductID, Quantity: remaining})
		}
	}
	return result, nil
}

type AllocationIdentity struct {
	Revision       uuid.UUID
	ActorID        uuid.UUID
	PlanID         uuid.UUID
	ItemID         uuid.UUID
	ShoppingListID uuid.UUID
	InventoryIDs   []uuid.UUID
}

type AllocationRepository interface {
	BindIdentity(identity AllocationIdentity) error
	Cancel(operationID uuid.UUID) (bool, error)
	Consume(operationID uuid.UUID) (bool, error)
	Allocate(
		operationID uuid.UUID,
		inventoryIDs []uuid.UUID,
		requirements []ReservationItem,
	) (AllocationResult, bool, error)
}

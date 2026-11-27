package repository

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"uuid"

	"inventory/internal/inventory/domain"
)

func (r *allocationRepository) Extend(
	operationID uuid.UUID,
	inventoryIDs []uuid.UUID,
	requirements []domain.ReservationItem,
) (domain.AllocationResult, bool, error) {
	if operationID == uuid.Nil() {
		return domain.AllocationResult{}, false, domain.ErrInvalidAllocation
	}

	request, hash, err := canonicalAllocationRequest(inventoryIDs, requirements)
	if err != nil {
		return domain.AllocationResult{}, false, err
	}

	current, _, err := r.existing(operationID, hash[:])
	if err != nil {
		return domain.AllocationResult{}, false, err
	}

	if len(current.Shortages) == 0 {
		return current, false, nil
	}

	stocks, err := r.lockStocks(request.Inventories, current.Shortages)
	if err != nil {
		return domain.AllocationResult{}, false, err
	}

	additional, err := domain.DistributePartial(request.Inventories, current.Shortages, stocks)
	if err != nil {
		return domain.AllocationResult{}, false, err
	}

	if len(additional.Lines) == 0 {
		return current, false, nil
	}

	if current.ResultVersion == ^uint64(0) {
		return domain.AllocationResult{}, false, domain.ErrInvalidAllocation
	}

	nextVersion := current.ResultVersion + 1
	grouped := make(map[uuid.UUID][]domain.ReservationItem)

	for index := range additional.Lines {
		line := &additional.Lines[index]

		line.BatchVersion = nextVersion
		line.ReservationID = allocationBatchReservationID(operationID, line.InventoryID, nextVersion)

		grouped[line.InventoryID] = append(
			grouped[line.InventoryID],
			domain.ReservationItem{
				ProductID: line.ProductID,
				Quantity:  line.Quantity,
			},
		)
	}

	reservations := NewReservationRepository(r.ctx, r.client)

	for _, inventoryID := range request.Inventories {
		items := grouped[inventoryID]
		if len(items) == 0 {
			continue
		}

		reservation, err := domain.NewReservation(
			allocationBatchReservationID(operationID, inventoryID, nextVersion),
			inventoryID,
			items,
		)
		if err != nil {
			return domain.AllocationResult{}, false, err
		}

		created, err := reservations.Reserve(reservation)
		if err != nil {
			return domain.AllocationResult{}, false, err
		}
		if !created {
			return domain.AllocationResult{}, false, errors.New("allocation batch already exists")
		}
	}

	current.Lines = append(current.Lines, additional.Lines...)
	current.Shortages = additional.Shortages
	current.ResultVersion = nextVersion

	payload, err := json.Marshal(current)
	if err != nil {
		return domain.AllocationResult{}, false, err
	}

	_, err = r.client.ExecContext(
		r.ctx,
		"allocation.extend",
		`UPDATE allocation_operations SET result = ? WHERE id = ?`,
		payload,
		operationID[:],
	)
	if err != nil {
		return domain.AllocationResult{}, false, fmt.Errorf("extend allocation: %w", err)
	}

	return current, true, nil
}

func (r *allocationRepository) lockStocks(
	inventoryIDs []uuid.UUID,
	requirements []domain.ReservationItem,
) ([]domain.Stock, error) {
	stocks := make([]domain.Stock, 0)

	for _, inventoryID := range inventoryIDs {
		for _, item := range requirements {
			stock, err := getStock(r.ctx, r.client, inventoryID, item.ProductID, true)
			if err != nil {
				return nil, err
			}
			if stock != nil {
				stocks = append(stocks, *stock)
			}
		}
	}

	return stocks, nil
}

func allocationBatchReservationID(
	operationID, inventoryID uuid.UUID,
	version uint64,
) uuid.UUID {
	if version == 1 {
		return allocationReservationID(operationID, inventoryID)
	}

	var input [40]byte

	copy(input[:16], operationID[:])
	copy(input[16:32], inventoryID[:])
	binary.BigEndian.PutUint64(input[32:], version)

	hash := sha256.Sum256(input[:])

	var id uuid.UUID
	copy(id[:], hash[:16])

	id[6] = id[6]&0x0f | 0x80
	id[8] = id[8]&0x3f | 0x80

	return id
}

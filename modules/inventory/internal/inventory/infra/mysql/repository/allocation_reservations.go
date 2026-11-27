package repository

import (
	"sort"
	"uuid"

	"inventory/internal/inventory/domain"
)

type allocationReservation struct {
	InventoryID   uuid.UUID
	ReservationID uuid.UUID
}

func allocationReservations(
	operationID uuid.UUID,
	result domain.AllocationResult,
) ([]allocationReservation, error) {
	normalizeAllocationResult(&result)

	reservations := make([]allocationReservation, 0)
	seen := make(map[allocationReservation]bool)

	for _, line := range result.Lines {
		if line.InventoryID == uuid.Nil() ||
			line.ReservationID == uuid.Nil() ||
			line.BatchVersion > result.ResultVersion {
			return nil, domain.ErrInvalidAllocation
		}

		expectedID := allocationBatchReservationID(
			operationID,
			line.InventoryID,
			line.BatchVersion,
		)

		if line.ReservationID != expectedID {
			return nil, domain.ErrInvalidAllocation
		}

		reservation := allocationReservation{
			InventoryID:   line.InventoryID,
			ReservationID: line.ReservationID,
		}

		if seen[reservation] {
			continue
		}

		seen[reservation] = true
		reservations = append(reservations, reservation)
	}

	sort.Slice(reservations, func(i, j int) bool {
		a, b := reservations[i], reservations[j]

		if a.InventoryID != b.InventoryID {
			return a.InventoryID.String() < b.InventoryID.String()
		}

		return a.ReservationID.String() < b.ReservationID.String()
	})

	return reservations, nil
}

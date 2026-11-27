package repository

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"uuid"

	"inventory/internal/inventory/domain"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	driver "github.com/go-sql-driver/mysql"
)

type allocationRepository struct {
	ctx    context.Context
	client mysql.ClientContext
}

func NewAllocationRepository(ctx context.Context, client mysql.ClientContext) domain.AllocationRepository {
	return &allocationRepository{ctx: ctx, client: client}
}

type allocationRequest struct {
	Inventories  []uuid.UUID
	Requirements []domain.ReservationItem
}

func (r *allocationRepository) Allocate(
	operationID uuid.UUID,
	inventoryIDs []uuid.UUID,
	requirements []domain.ReservationItem,
) (domain.AllocationResult, bool, error) {
	if operationID == uuid.Nil() {
		return domain.AllocationResult{}, false, domain.ErrInvalidAllocation
	}
	if _, err := domain.DistributePartial(inventoryIDs, requirements, nil); err != nil {
		return domain.AllocationResult{}, false, err
	}
	request := allocationRequest{
		Inventories:  append([]uuid.UUID(nil), inventoryIDs...),
		Requirements: append([]domain.ReservationItem(nil), requirements...),
	}
	sort.Slice(
		request.Inventories,
		func(i, j int) bool { return request.Inventories[i].String() < request.Inventories[j].String() },
	)
	sort.Slice(request.Requirements, func(i, j int) bool {
		return request.Requirements[i].ProductID.String() < request.Requirements[j].ProductID.String()
	})
	encoded, err := json.Marshal(request)
	if err != nil {
		return domain.AllocationResult{}, false, err
	}
	hash := sha256.Sum256(encoded)
	_, err = r.client.ExecContext(
		r.ctx,
		"allocation.insert",
		`INSERT INTO allocation_operations (id, request_hash) VALUES (?, ?)`,
		operationID[:],
		hash[:],
	)
	var mysqlErr *driver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return r.existing(operationID, hash[:])
	}
	if err != nil {
		return domain.AllocationResult{}, false, fmt.Errorf("insert allocation: %w", err)
	}
	stocks := make([]domain.Stock, 0)
	for _, inventoryID := range request.Inventories {
		for _, item := range request.Requirements {
			stock, err := getStock(r.ctx, r.client, inventoryID, item.ProductID, true)
			if err != nil {
				return domain.AllocationResult{}, false, err
			}
			if stock != nil {
				stocks = append(stocks, *stock)
			}
		}
	}
	result, err := domain.DistributePartial(request.Inventories, request.Requirements, stocks)
	if err != nil {
		return domain.AllocationResult{}, false, err
	}
	for index := range result.Lines {
		result.Lines[index].ReservationID = allocationReservationID(operationID, result.Lines[index].InventoryID)
	}
	grouped := make(map[uuid.UUID][]domain.ReservationItem)
	for _, line := range result.Lines {
		grouped[line.InventoryID] = append(
			grouped[line.InventoryID],
			domain.ReservationItem{ProductID: line.ProductID, Quantity: line.Quantity},
		)
	}
	for _, inventoryID := range request.Inventories {
		if len(grouped[inventoryID]) == 0 {
			continue
		}
		reservation, err := domain.NewReservation(
			allocationReservationID(operationID, inventoryID),
			inventoryID,
			grouped[inventoryID],
		)
		if err != nil {
			return domain.AllocationResult{}, false, err
		}
		created, err := NewReservationRepository(r.ctx, r.client).Reserve(reservation)
		if err != nil {
			return domain.AllocationResult{}, false, err
		}
		if !created {
			return domain.AllocationResult{}, false, errors.New("allocation reservation already exists")
		}
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return domain.AllocationResult{}, false, err
	}
	_, err = r.client.ExecContext(
		r.ctx,
		"allocation.complete",
		`UPDATE allocation_operations SET result = ? WHERE id = ?`,
		payload,
		operationID[:],
	)
	if err != nil {
		return domain.AllocationResult{}, false, fmt.Errorf("complete allocation: %w", err)
	}
	return result, true, nil
}
func (r *allocationRepository) existing(id uuid.UUID, expected []byte) (domain.AllocationResult, bool, error) {
	var hash, payload []byte
	var cancelled bool
	err := r.client.QueryRowContext(r.ctx, "allocation.existing", `SELECT request_hash, result, cancelled FROM allocation_operations WHERE id = ? FOR UPDATE`, id[:]).
		Scan(&hash, &payload, &cancelled)
	if err != nil {
		return domain.AllocationResult{}, false, fmt.Errorf("get allocation: %w", err)
	}
	if cancelled && len(hash) == 0 {
		return domain.AllocationResult{}, false, domain.ErrAllocationCancelled
	}
	if !bytes.Equal(hash, expected) {
		return domain.AllocationResult{}, false, domain.ErrAllocationConflict
	}
	if cancelled {
		return domain.AllocationResult{}, false, domain.ErrAllocationCancelled
	}
	if len(payload) == 0 {
		return domain.AllocationResult{}, false, errors.New("incomplete allocation")
	}
	var result domain.AllocationResult
	if err := json.Unmarshal(payload, &result); err != nil {
		return result, false, fmt.Errorf("decode allocation: %w", err)
	}
	return result, false, nil
}
func allocationReservationID(operationID, inventoryID uuid.UUID) uuid.UUID {
	var input [32]byte
	copy(input[:16], operationID[:])
	copy(input[16:], inventoryID[:])
	hash := sha256.Sum256(input[:])
	var id uuid.UUID
	copy(id[:], hash[:16])
	id[6] = id[6]&0x0f | 0x80
	id[8] = id[8]&0x3f | 0x80
	return id
}

// Cancel requires a unit of work and serializes against Allocate on the operation row.
func (r *allocationRepository) Cancel(operationID uuid.UUID) (bool, error) {
	if operationID == uuid.Nil() {
		return false, domain.ErrInvalidAllocation
	}
	_, err := r.client.ExecContext(
		r.ctx,
		"allocation.cancel_marker",
		`INSERT INTO allocation_operations (id, request_hash, result, cancelled) VALUES (?, NULL, NULL, TRUE)`,
		operationID[:],
	)
	var mysqlErr *driver.MySQLError
	if err == nil {
		return true, nil
	}
	if !errors.As(err, &mysqlErr) || mysqlErr.Number != 1062 {
		return false, fmt.Errorf("insert cancellation marker: %w", err)
	}
	var payload []byte
	var cancelled bool
	err = r.client.QueryRowContext(r.ctx, "allocation.lock_for_cancel", `SELECT result, cancelled FROM allocation_operations WHERE id = ? FOR UPDATE`, operationID[:]).
		Scan(&payload, &cancelled)
	if err != nil {
		return false, fmt.Errorf("lock allocation for cancellation: %w", err)
	}
	if cancelled {
		return false, nil
	}
	if len(payload) == 0 {
		return false, errors.New("incomplete allocation")
	}
	var result domain.AllocationResult
	if err := json.Unmarshal(payload, &result); err != nil {
		return false, fmt.Errorf("decode allocation for cancellation: %w", err)
	}
	reservations := make(map[uuid.UUID]uuid.UUID)
	for _, line := range result.Lines {
		if existing, ok := reservations[line.InventoryID]; ok && existing != line.ReservationID {
			return false, domain.ErrInvalidAllocation
		}
		if line.ReservationID != allocationReservationID(operationID, line.InventoryID) {
			return false, domain.ErrInvalidAllocation
		}
		reservations[line.InventoryID] = line.ReservationID
	}
	inventories := make([]uuid.UUID, 0, len(reservations))
	for inventoryID := range reservations {
		inventories = append(inventories, inventoryID)
	}
	sort.Slice(inventories, func(i, j int) bool { return inventories[i].String() < inventories[j].String() })
	for _, inventoryID := range inventories {
		if _, err := NewReservationRepository(
			r.ctx,
			r.client,
		).Release(inventoryID, reservations[inventoryID]); err != nil {
			return false, fmt.Errorf("release allocation reservation: %w", err)
		}
	}
	if _, err := r.client.ExecContext(
		r.ctx,
		"allocation.cancel",
		`UPDATE allocation_operations SET cancelled = TRUE WHERE id = ?`,
		operationID[:],
	); err != nil {
		return false, fmt.Errorf("cancel allocation: %w", err)
	}
	return true, nil
}

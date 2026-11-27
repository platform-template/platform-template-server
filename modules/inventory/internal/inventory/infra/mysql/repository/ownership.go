package repository

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"

	"inventory/internal/inventory/domain"
)

type ownershipRepository struct {
	ctx    context.Context
	client mysql.ClientContext
}

func NewOwnershipRepository(ctx context.Context, client mysql.ClientContext) domain.InventoryOwnership {
	return &ownershipRepository{ctx: ctx, client: client}
}

func (r *ownershipRepository) Create(inventoryID, userID uuid.UUID) error {
	if inventoryID == uuid.Nil() || userID == uuid.Nil() {
		return domain.ErrInvalidInventory
	}
	_, err := r.client.ExecContext(
		r.ctx,
		"inventory.owner.create",
		`INSERT INTO inventory_owners (inventory_id, user_id) VALUES (?, ?)`,
		inventoryID[:],
		userID[:],
	)
	if err != nil {
		return fmt.Errorf("create inventory owner: %w", err)
	}
	return nil
}

func (r *ownershipRepository) IsOwner(inventoryID, userID uuid.UUID) (bool, error) {
	if inventoryID == uuid.Nil() || userID == uuid.Nil() {
		return false, nil
	}
	var count int
	err := r.client.QueryRowContext(r.ctx, "inventory.owner.exists", `SELECT COUNT(*) FROM inventory_owners WHERE inventory_id = ? AND user_id = ?`, inventoryID[:], userID[:]).
		Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check inventory owner: %w", err)
	}
	return count != 0, nil
}

func (r *ownershipRepository) ListByUser(userID uuid.UUID) ([]*domain.Inventory, error) {
	rows, err := r.client.QueryContext(
		r.ctx,
		"inventory.owner.list",
		`SELECT i.id, i.name FROM inventories i JOIN inventory_owners o ON o.inventory_id = i.id WHERE o.user_id = ? ORDER BY i.name, i.id`,
		userID[:],
	)
	if err != nil {
		return nil, fmt.Errorf("list owned inventories: %w", err)
	}
	defer rows.Close()
	inventories := make([]*domain.Inventory, 0)
	for rows.Next() {
		var bytes []byte
		var name string
		if err := rows.Scan(&bytes, &name); err != nil {
			return nil, fmt.Errorf("scan inventory: %w", err)
		}
		if len(bytes) != 16 {
			return nil, errors.New("invalid inventory ID")
		}
		var id uuid.UUID
		copy(id[:], bytes)
		inventory, err := domain.NewInventory(id, name)
		if err != nil {
			return nil, fmt.Errorf("restore inventory: %w", err)
		}
		inventories = append(inventories, inventory)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate inventories: %w", err)
	}
	return inventories, nil
}

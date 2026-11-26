package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"unicode/utf8"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	driver "github.com/go-sql-driver/mysql"

	"inventory/internal/inventory/domain"
)

type inventoryRepository struct {
	ctx    context.Context
	client mysql.ClientContext
}

func NewInventoryRepository(ctx context.Context, client mysql.ClientContext) domain.InventoryRepository {
	return &inventoryRepository{ctx: ctx, client: client}
}

func (r *inventoryRepository) Create(inventory *domain.Inventory) error {
	if err := validateInventory(inventory); err != nil {
		return err
	}
	id := inventory.ID()
	_, err := r.client.ExecContext(
		r.ctx,
		"inventory.create",
		`INSERT INTO inventories (id, name) VALUES (?, ?)`,
		id[:],
		inventory.Name(),
	)
	var mysqlErr *driver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return domain.ErrInventoryExists
	}
	if err != nil {
		return fmt.Errorf("create inventory: %w", err)
	}
	return nil
}

func (r *inventoryRepository) Update(inventory *domain.Inventory) error {
	if err := validateInventory(inventory); err != nil {
		return err
	}
	id := inventory.ID()
	result, err := r.client.ExecContext(
		r.ctx,
		"inventory.update",
		`UPDATE inventories SET name = ? WHERE id = ?`,
		inventory.Name(),
		id[:],
	)
	if err != nil {
		return fmt.Errorf("update inventory: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count updated inventories: %w", err)
	}
	if count == 0 {
		existing, err := r.GetByID(id)
		if err != nil {
			return err
		}
		if existing == nil {
			return domain.ErrInventoryNotFound
		}
	}
	return nil
}

func (r *inventoryRepository) GetByID(id uuid.UUID) (*domain.Inventory, error) {
	var name string
	err := r.client.QueryRowContext(r.ctx, "inventory.get", `SELECT name FROM inventories WHERE id = ?`, id[:]).
		Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get inventory: %w", err)
	}
	inventory, err := domain.NewInventory(id, name)
	if err != nil {
		return nil, fmt.Errorf("restore inventory: %w", err)
	}
	return inventory, nil
}

func (r *inventoryRepository) List() ([]*domain.Inventory, error) {
	rows, err := r.client.QueryContext(r.ctx, "inventory.list", `SELECT id, name FROM inventories ORDER BY name, id`)
	if err != nil {
		return nil, fmt.Errorf("list inventories: %w", err)
	}
	defer rows.Close()
	inventories := make([]*domain.Inventory, 0)
	for rows.Next() {
		var idBytes []byte
		var name string
		if err := rows.Scan(&idBytes, &name); err != nil {
			return nil, fmt.Errorf("scan inventory: %w", err)
		}
		if len(idBytes) != 16 {
			return nil, errors.New("invalid inventory ID")
		}
		var id uuid.UUID
		copy(id[:], idBytes)
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

func validateInventory(inventory *domain.Inventory) error {
	if inventory == nil {
		return domain.ErrInvalidInventory
	}
	if _, err := domain.NewInventory(inventory.ID(), inventory.Name()); err != nil {
		return err
	}
	if !utf8.ValidString(inventory.Name()) || utf8.RuneCountInString(inventory.Name()) > 255 {
		return fmt.Errorf("inventory name exceeds storage constraints")
	}
	return nil
}

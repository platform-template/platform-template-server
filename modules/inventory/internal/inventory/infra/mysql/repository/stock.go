package repository

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	driver "github.com/go-sql-driver/mysql"

	"inventory/internal/inventory/domain"
)

type stockRepository struct {
	ctx    context.Context
	client mysql.ClientContext
}

const forUpdate = ` FOR UPDATE`

func NewStockRepository(ctx context.Context, client mysql.ClientContext) domain.StockRepository {
	return &stockRepository{ctx: ctx, client: client}
}

func (r *stockRepository) Create(stock domain.Stock) error {
	if _, err := domain.NewStock(
		stock.InventoryID,
		stock.ProductID,
		stock.Quantity,
		stock.ReservedQuantity,
	); err != nil {
		return err
	}
	_, err := r.client.ExecContext(r.ctx, "stock.create", `INSERT INTO stocks
		(inventory_id, product_id, quantity, reserved_quantity) VALUES (?, ?, ?, ?)`,
		stock.InventoryID[:], stock.ProductID[:], stock.Quantity, stock.ReservedQuantity)
	var mysqlErr *driver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return domain.ErrStockExists
	}
	if err != nil {
		return fmt.Errorf("create stock: %w", err)
	}
	return nil
}

func (r *stockRepository) GetByProductID(inventoryID, productID uuid.UUID) (*domain.Stock, error) {
	return getStock(r.ctx, r.client, inventoryID, productID, false)
}

func (r *stockRepository) GetByProductIDs(inventoryID uuid.UUID, ids []uuid.UUID) ([]domain.Stock, error) {
	stocks := make([]domain.Stock, 0, len(ids))
	for _, id := range ids {
		stock, err := r.GetByProductID(inventoryID, id)
		if err != nil {
			return nil, err
		}
		if stock != nil {
			stocks = append(stocks, *stock)
		}
	}
	return stocks, nil
}

func (r *stockRepository) List(inventoryID uuid.UUID) ([]domain.Stock, error) {
	rows, err := r.client.QueryContext(r.ctx, "stock.list", `SELECT product_id, quantity, reserved_quantity
		FROM stocks WHERE inventory_id = ? ORDER BY product_id`, inventoryID[:])
	if err != nil {
		return nil, fmt.Errorf("list stocks: %w", err)
	}
	defer rows.Close()
	stocks := make([]domain.Stock, 0)
	for rows.Next() {
		var id []byte
		var quantity, reserved int64
		if err := rows.Scan(&id, &quantity, &reserved); err != nil {
			return nil, fmt.Errorf("scan stock: %w", err)
		}
		if len(id) != 16 {
			return nil, errors.New("invalid product ID")
		}
		var productID uuid.UUID
		copy(productID[:], id)
		stock, err := domain.NewStock(inventoryID, productID, quantity, reserved)
		if err != nil {
			return nil, fmt.Errorf("restore stock: %w", err)
		}
		stocks = append(stocks, stock)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stocks: %w", err)
	}
	return stocks, nil
}

func (r *stockRepository) Add(inventoryID, productID uuid.UUID, quantity int64) error {
	if inventoryID == uuid.Nil() || productID == uuid.Nil() || quantity <= 0 {
		return domain.ErrInvalidStock
	}
	_, err := r.client.ExecContext(r.ctx, "stock.add", `INSERT INTO stocks
		(inventory_id, product_id, quantity, reserved_quantity) VALUES (?, ?, ?, 0)
		ON DUPLICATE KEY UPDATE quantity = quantity + VALUES(quantity)`, inventoryID[:], productID[:], quantity)
	var mysqlErr *driver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1690 {
		return domain.ErrInvalidStock
	}
	if err != nil {
		return fmt.Errorf("add stock: %w", err)
	}
	return nil
}

func (r *stockRepository) Replenish(operationID, inventoryID, productID uuid.UUID, quantity int64) (bool, error) {
	if operationID == uuid.Nil() || inventoryID == uuid.Nil() || productID == uuid.Nil() || quantity <= 0 {
		return false, domain.ErrInvalidStock
	}
	_, err := r.client.ExecContext(r.ctx, "stock.replenishment.insert", `INSERT INTO stock_replenishment_operations (id, inventory_id, product_id, quantity) VALUES (?, ?, ?, ?)`, operationID[:], inventoryID[:], productID[:], quantity)
	var mysqlErr *driver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		var existingInventory, existingProduct []byte
		var existingQuantity int64
		if err := r.client.QueryRowContext(r.ctx, "stock.replenishment.lock", `SELECT inventory_id, product_id, quantity FROM stock_replenishment_operations WHERE id = ? FOR UPDATE`, operationID[:]).Scan(&existingInventory, &existingProduct, &existingQuantity); err != nil {
			return false, fmt.Errorf("get replenishment: %w", err)
		}
		if !bytes.Equal(existingInventory, inventoryID[:]) || !bytes.Equal(existingProduct, productID[:]) || existingQuantity != quantity {
			return false, domain.ErrInvalidStock
		}
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("record replenishment: %w", err)
	}
	if err := r.Add(inventoryID, productID, quantity); err != nil {
		return false, err
	}
	return true, nil
}

func (r *stockRepository) Remove(inventoryID, productID uuid.UUID, quantity int64) error {
	if inventoryID == uuid.Nil() || productID == uuid.Nil() || quantity <= 0 {
		return domain.ErrInvalidQuantityToRemove
	}
	result, err := r.client.ExecContext(r.ctx, "stock.remove", `UPDATE stocks SET quantity = quantity - ?
		WHERE inventory_id = ? AND product_id = ? AND quantity - reserved_quantity >= ?`,
		quantity, inventoryID[:], productID[:], quantity)
	if err != nil {
		return fmt.Errorf("remove stock: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count removed stock: %w", err)
	}
	if count == 0 {
		return domain.ErrInsufficientStock
	}
	return nil
}

func (r *stockRepository) Delete(inventoryID, productID uuid.UUID) error {
	result, err := r.client.ExecContext(r.ctx, "stock.delete", `DELETE FROM stocks
		WHERE inventory_id = ? AND product_id = ? AND quantity = 0 AND reserved_quantity = 0`, inventoryID[:], productID[:])
	if err != nil {
		return fmt.Errorf("delete stock: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count deleted stock: %w", err)
	}
	if count == 0 {
		stock, err := r.GetByProductID(inventoryID, productID)
		if err != nil {
			return err
		}
		if stock != nil {
			return domain.ErrInvalidStock
		}
	}
	return nil
}

func getStock(
	ctx context.Context,
	client mysql.ClientContext,
	inventoryID, productID uuid.UUID,
	lock bool,
) (*domain.Stock, error) {
	query := `SELECT quantity, reserved_quantity FROM stocks WHERE inventory_id = ? AND product_id = ?`
	if lock {
		query += forUpdate
	}
	var quantity, reserved int64
	err := client.QueryRowContext(ctx, "stock.get", query, inventoryID[:], productID[:]).Scan(&quantity, &reserved)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get stock: %w", err)
	}
	stock, err := domain.NewStock(inventoryID, productID, quantity, reserved)
	if err != nil {
		return nil, fmt.Errorf("restore stock: %w", err)
	}
	return &stock, nil
}

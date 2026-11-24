package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"

	"inventory/internal/inventory/domain"
)

type stockRepository struct {
	ctx    context.Context
	client mysql.ClientContext
}

func NewStockRepository(ctx context.Context, client mysql.ClientContext) domain.StockRepository {
	return &stockRepository{
		ctx:    ctx,
		client: client,
	}
}

func (r *stockRepository) GetByProductID(id uuid.UUID) (*domain.Stock, error) {
	return getStock(r.ctx, r.client, id, false)
}

func (r *stockRepository) GetByProductIDs(ids []uuid.UUID) ([]domain.Stock, error) {
	stocks := make([]domain.Stock, 0, len(ids))
	for _, id := range ids {
		stock, err := r.GetByProductID(id)
		if err != nil {
			return nil, err
		}
		if stock != nil {
			stocks = append(stocks, *stock)
		}
	}
	return stocks, nil
}

func (r *stockRepository) Add(id uuid.UUID, quantity int64) error {
	if id == uuid.Nil() || quantity <= 0 {
		return domain.ErrInvalidStock
	}
	result, err := r.client.ExecContext(
		r.ctx,
		"stock.add",
		`UPDATE stocks SET quantity = quantity + ? WHERE product_id = ? AND quantity <= ?`,
		quantity,
		id[:],
		int64(math.MaxInt64)-quantity,
	)
	if err != nil {
		return fmt.Errorf("add stock: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("count updated stock: %w", err)
	}
	if count == 0 {
		stock, err := r.GetByProductID(id)
		if err != nil {
			return err
		}
		if stock == nil {
			return domain.ErrProductNotFound
		}
		return domain.ErrInvalidStock
	}
	return nil
}

func getStock(ctx context.Context, client mysql.ClientContext, id uuid.UUID, lock bool) (*domain.Stock, error) {
	query := `SELECT quantity, reserved_quantity FROM stocks WHERE product_id = ?`
	if lock {
		query += ` FOR UPDATE`
	}
	var quantity, reserved int64
	err := client.QueryRowContext(ctx, "stock.get", query, id[:]).Scan(&quantity, &reserved)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get stock: %w", err)
	}
	stock, err := domain.NewStock(id, quantity, reserved)
	if err != nil {
		return nil, fmt.Errorf("restore stock: %w", err)
	}
	return &stock, nil
}

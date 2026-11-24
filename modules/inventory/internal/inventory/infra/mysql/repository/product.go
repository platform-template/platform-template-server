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

type productRepository struct {
	ctx    context.Context
	client mysql.ClientContext
}

func NewProductRepository(ctx context.Context, client mysql.ClientContext) domain.ProductRepository {
	return &productRepository{
		ctx:    ctx,
		client: client,
	}
}

func (r *productRepository) Create(product domain.Product) error {
	if _, err := domain.NewProduct(product.ID, product.Name, product.Unit); err != nil {
		return err
	}
	if !utf8.ValidString(product.Name) || utf8.RuneCountInString(product.Name) > 255 {
		return fmt.Errorf("product name exceeds storage constraints")
	}
	_, err := r.client.ExecContext(
		r.ctx,
		"product.create",
		`INSERT INTO products (id, name, unit) VALUES (?, ?, ?)`,
		product.ID[:], product.Name, string(product.Unit),
	)
	var mysqlErr *driver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return domain.ErrProductExists
	}
	if err != nil {
		return fmt.Errorf("create product: %w", err)
	}
	_, err = r.client.ExecContext(
		r.ctx,
		"stock.create",
		`INSERT INTO stocks (product_id, quantity, reserved_quantity) VALUES (?, 0, 0)`,
		product.ID[:],
	)
	if err != nil {
		return fmt.Errorf("create stock: %w", err)
	}
	return nil
}

func (r *productRepository) GetByID(id uuid.UUID) (*domain.Product, error) {
	var name, unit string
	err := r.client.QueryRowContext(
		r.ctx,
		"product.get",
		`SELECT name, unit FROM products WHERE id = ?`,
		id[:],
	).Scan(&name, &unit)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get product: %w", err)
	}
	product, err := domain.NewProduct(id, name, domain.Unit(unit))
	if err != nil {
		return nil, fmt.Errorf("restore product: %w", err)
	}
	return &product, nil
}

func (r *productRepository) List() ([]domain.Product, error) {
	rows, err := r.client.QueryContext(
		r.ctx,
		"product.list",
		`SELECT id, name, unit FROM products ORDER BY id`,
	)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	defer rows.Close()
	products := make([]domain.Product, 0)
	for rows.Next() {
		var id []byte
		var name, unit string
		if err := rows.Scan(&id, &name, &unit); err != nil {
			return nil, fmt.Errorf("scan product: %w", err)
		}
		if len(id) != 16 {
			return nil, errors.New("invalid product ID")
		}
		var productID uuid.UUID
		copy(productID[:], id)
		product, err := domain.NewProduct(productID, name, domain.Unit(unit))
		if err != nil {
			return nil, fmt.Errorf("restore product: %w", err)
		}
		products = append(products, product)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate products: %w", err)
	}
	return products, nil
}

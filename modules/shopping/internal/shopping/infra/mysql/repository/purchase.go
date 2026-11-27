package repository

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"uuid"

	"shopping/internal/shopping/domain"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	driver "github.com/go-sql-driver/mysql"
)

type purchaseRepository struct {
	ctx    context.Context
	client mysql.ClientContext
}

func NewPurchaseRepository(ctx context.Context, client mysql.ClientContext) domain.PurchaseRepository {
	return &purchaseRepository{ctx: ctx, client: client}
}

func (r *purchaseRepository) Create(listID uuid.UUID, purchase *domain.Purchase) (bool, error) {
	data := purchase.Data()
	_, err := r.client.ExecContext(
		r.ctx,
		"purchase.create",
		`INSERT INTO shopping_purchases (id,list_id,demand_id,product_id,inventory_id,quantity,confirmed) VALUES (?, ?, ?, ?, ?, ?, TRUE)`,
		data.ID[:],
		listID[:],
		data.DemandID[:],
		data.ProductID[:],
		data.InventoryID[:],
		data.Quantity,
	)
	var mysqlErr *driver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		var demandID, productID, inventoryID []byte
		var quantity int64
		if err := r.client.QueryRowContext(
			r.ctx,
			"purchase.get",
			`SELECT demand_id,product_id,inventory_id,quantity FROM shopping_purchases WHERE id = ? FOR UPDATE`,
			data.ID[:],
		).Scan(&demandID, &productID, &inventoryID, &quantity); err != nil {
			return false, fmt.Errorf("get duplicate purchase: %w", err)
		}
		if !bytes.Equal(demandID, data.DemandID[:]) ||
			!bytes.Equal(productID, data.ProductID[:]) ||
			!bytes.Equal(inventoryID, data.InventoryID[:]) ||
			quantity != data.Quantity {
			return false, domain.ErrDemandConflict
		}
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create purchase: %w", err)
	}
	return true, nil
}

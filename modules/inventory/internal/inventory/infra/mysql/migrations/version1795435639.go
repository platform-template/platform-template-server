package migrations

import (
	"context"

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

func Version1795435639(client mysql.ClientContext) migrator.Migration {
	return &version1795435639{
		client: client,
	}
}

type version1795435639 struct {
	client mysql.ClientContext
}

func (v *version1795435639) Version() int64 {
	return 1795435639
}

func (v *version1795435639) Description() string {
	return "Create inventory storage"
}

func (v *version1795435639) Up(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS products (
id BINARY(16) PRIMARY KEY,
name VARCHAR(255) NOT NULL,
unit VARCHAR(3) NOT NULL,
CHECK (unit IN ('g', 'ml', 'pcs'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS stocks (
product_id BINARY(16) PRIMARY KEY,
quantity BIGINT NOT NULL,
reserved_quantity BIGINT NOT NULL,
CHECK (quantity >= 0 AND reserved_quantity >= 0 AND reserved_quantity <= quantity),
FOREIGN KEY (product_id) REFERENCES products(id)
) ENGINE=InnoDB`,
		`CREATE TABLE IF NOT EXISTS reservations (
id BINARY(16) PRIMARY KEY,
status VARCHAR(16) NOT NULL,
CHECK (status IN ('active', 'cancelled'))
) ENGINE=InnoDB`,
		`CREATE TABLE IF NOT EXISTS reservation_items (
reservation_id BINARY(16) NOT NULL,
product_id BINARY(16) NOT NULL,
quantity BIGINT NOT NULL,
PRIMARY KEY (reservation_id, product_id),
KEY reservation_items_product (product_id),
CHECK (quantity > 0),
FOREIGN KEY (reservation_id) REFERENCES reservations(id),
FOREIGN KEY (product_id) REFERENCES products(id)
) ENGINE=InnoDB`,
	}
	for _, statement := range statements {
		if _, err := v.client.ExecContext(ctx, "inventory.create_schema", statement); err != nil {
			return err
		}
	}
	return nil
}

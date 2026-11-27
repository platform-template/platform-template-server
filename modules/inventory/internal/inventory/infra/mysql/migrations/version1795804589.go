package migrations

import (
	"context"
	"fmt"

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

func Version1795804589(client mysql.ClientContext) migrator.Migration {
	return &version1795804589{
		client: client,
	}
}

type version1795804589 struct {
	client mysql.ClientContext
}

func (v *version1795804589) Version() int64 {
	return 1795804589
}

func (v *version1795804589) Description() string {
	return "Record stock replenishment operations"
}

func (v *version1795804589) Up(ctx context.Context) error {
	_, err := v.client.ExecContext(ctx, "stock.create_replenishments", `CREATE TABLE IF NOT EXISTS stock_replenishment_operations (
 id BINARY(16) NOT NULL PRIMARY KEY, inventory_id BINARY(16) NOT NULL, product_id BINARY(16) NOT NULL, quantity BIGINT NOT NULL,
 CHECK (quantity > 0)
 ) ENGINE=InnoDB`)
	if err != nil {
		return fmt.Errorf("create stock replenishment operations: %w", err)
	}
	return nil
}

package migrations

import (
	"context"
	"fmt"

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

func Version1795764223(client mysql.ClientContext) migrator.Migration {
	return &version1795764223{
		client: client,
	}
}

type version1795764223 struct {
	client mysql.ClientContext
}

func (v *version1795764223) Version() int64 {
	return 1795764223
}

func (v *version1795764223) Description() string {
	return "Associate inventories with creators"
}

func (v *version1795764223) Up(ctx context.Context) error {
	_, err := v.client.ExecContext(ctx, "inventory.create_owners", `CREATE TABLE IF NOT EXISTS inventory_owners (
		inventory_id BINARY(16) NOT NULL PRIMARY KEY,
		user_id BINARY(16) NOT NULL,
		KEY inventory_owners_user (user_id, inventory_id),
		CONSTRAINT inventory_owners_inventory_fk FOREIGN KEY (inventory_id) REFERENCES inventories(id)
	) ENGINE=InnoDB`)
	if err != nil {
		return fmt.Errorf("create inventory owners: %w", err)
	}
	return nil
}

package migrations

import (
	"context"
	"fmt"

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

func Version1795732885(client mysql.ClientContext) migrator.Migration {
	return &version1795732885{
		client: client,
	}
}

type version1795732885 struct {
	client mysql.ClientContext
}

func (v *version1795732885) Version() int64 {
	return 1795732885
}

func (v *version1795732885) Description() string {
	return "Support multiple inventories"
}

// MySQL DDL commits implicitly, so each schema step can be retried after interruption.
func (v *version1795732885) Up(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS inventories (
			id BINARY(16) PRIMARY KEY, name VARCHAR(255) NOT NULL
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`INSERT IGNORE INTO inventories (id, name)
		 SELECT UNHEX('00000000000000000000000000000001'), 'Default'
		 WHERE EXISTS (SELECT 1 FROM stocks) OR EXISTS (SELECT 1 FROM reservations)`,
	}
	for _, statement := range statements {
		if _, err := v.client.ExecContext(ctx, "inventory.multiple.prepare", statement); err != nil {
			return fmt.Errorf("prepare inventories: %w", err)
		}
	}
	steps := []struct{ query, ddl string }{
		{
			`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'stocks' AND column_name = 'inventory_id'`,
			`ALTER TABLE stocks ADD COLUMN inventory_id BINARY(16) NULL`,
		},
		{
			`SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'reservations' AND column_name = 'inventory_id'`,
			`ALTER TABLE reservations ADD COLUMN inventory_id BINARY(16) NULL`,
		},
	}
	for _, step := range steps {
		if err := v.ensure(ctx, step.query, step.ddl); err != nil {
			return err
		}
	}
	for _, table := range []string{"stocks", "reservations"} {
		query := `UPDATE ` + table + ` SET inventory_id = UNHEX('00000000000000000000000000000001') WHERE inventory_id IS NULL`
		if _, err := v.client.ExecContext(ctx, "inventory.multiple.backfill", query); err != nil {
			return fmt.Errorf("backfill %s: %w", table, err)
		}
		query = `ALTER TABLE ` + table + ` MODIFY inventory_id BINARY(16) NOT NULL`
		if _, err := v.client.ExecContext(ctx, "inventory.multiple.required", query); err != nil {
			return fmt.Errorf("require %s inventory: %w", table, err)
		}
	}
	steps = []struct{ query, ddl string }{
		{
			`SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = 'stocks' AND index_name = 'stocks_product_idx'`,
			`ALTER TABLE stocks ADD KEY stocks_product_idx (product_id)`,
		},
		{
			`SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = DATABASE() AND table_name = 'stocks' AND index_name = 'PRIMARY' AND column_name = 'inventory_id'`,
			`ALTER TABLE stocks DROP PRIMARY KEY, ADD PRIMARY KEY (inventory_id, product_id)`,
		},
		{
			`SELECT COUNT(*) FROM information_schema.table_constraints WHERE constraint_schema = DATABASE() AND table_name = 'stocks' AND constraint_name = 'stocks_inventory_fk'`,
			`ALTER TABLE stocks ADD CONSTRAINT stocks_inventory_fk FOREIGN KEY (inventory_id) REFERENCES inventories(id)`,
		},
		{
			`SELECT COUNT(*) FROM information_schema.table_constraints WHERE constraint_schema = DATABASE() AND table_name = 'reservations' AND constraint_name = 'reservations_inventory_fk'`,
			`ALTER TABLE reservations ADD CONSTRAINT reservations_inventory_fk FOREIGN KEY (inventory_id) REFERENCES inventories(id)`,
		},
	}
	for _, step := range steps {
		if err := v.ensure(ctx, step.query, step.ddl); err != nil {
			return err
		}
	}
	return nil
}

func (v *version1795732885) ensure(ctx context.Context, query, ddl string) error {
	var count int
	if err := v.client.QueryRowContext(ctx, "inventory.multiple.inspect", query).Scan(&count); err != nil {
		return fmt.Errorf("inspect inventory schema: %w", err)
	}
	if count != 0 {
		return nil
	}
	if _, err := v.client.ExecContext(ctx, "inventory.multiple.alter", ddl); err != nil {
		return fmt.Errorf("alter inventory schema: %w", err)
	}
	return nil
}

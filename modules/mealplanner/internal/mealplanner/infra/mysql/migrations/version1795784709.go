package migrations

import (
	"context"
	"fmt"

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

func Version1795784709(client mysql.ClientContext) migrator.Migration {
	return &version1795784709{
		client: client,
	}
}

type version1795784709 struct {
	client mysql.ClientContext
}

func (v *version1795784709) Version() int64 {
	return 1795784709
}

func (v *version1795784709) Description() string {
	return "Create meal plans and items"
}

func (v *version1795784709) Up(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS meal_plans (
    id BINARY(16) NOT NULL PRIMARY KEY, owner_id BINARY(16) NOT NULL,
    name VARCHAR(255) NOT NULL, KEY meal_plans_owner (owner_id, id)
   ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS meal_items (
    id BINARY(16) NOT NULL PRIMARY KEY, plan_id BINARY(16) NOT NULL,
    scheduled_at DATETIME(6) NOT NULL, payload JSON NOT NULL, row_version BIGINT UNSIGNED NOT NULL,
    KEY meal_items_calendar (plan_id, scheduled_at, id),
    CONSTRAINT meal_items_plan_fk FOREIGN KEY (plan_id) REFERENCES meal_plans(id)
   ) ENGINE=InnoDB`,
	}
	for _, statement := range statements {
		if _, err := v.client.ExecContext(ctx, "mealplanner.create_schema", statement); err != nil {
			return fmt.Errorf("create meal planner schema: %w", err)
		}
	}
	return nil
}

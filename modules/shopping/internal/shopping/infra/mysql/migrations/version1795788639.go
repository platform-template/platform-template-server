package migrations

import (
	"context"
	"fmt"

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

func Version1795788639(client mysql.ClientContext) migrator.Migration {
	return &version1795788639{
		client: client,
	}
}

type version1795788639 struct {
	client mysql.ClientContext
}

func (v *version1795788639) Version() int64 {
	return 1795788639
}

func (v *version1795788639) Description() string {
	return "Create shopping demands"
}

func (v *version1795788639) Up(ctx context.Context) error {
	_, err := v.client.ExecContext(ctx, "shopping.create_demands", `CREATE TABLE IF NOT EXISTS shopping_demands (
 id BINARY(16) NOT NULL PRIMARY KEY,
 list_id BINARY(16) NOT NULL, product_id BINARY(16) NOT NULL,
 meal_plan_id BINARY(16) NULL, meal_item_id BINARY(16) NULL, revision BINARY(16) NULL,
 required BIGINT NOT NULL, purchased BIGINT NOT NULL DEFAULT 0, cancelled BOOLEAN NOT NULL DEFAULT FALSE,
 UNIQUE KEY shopping_demand_source (list_id, meal_plan_id, meal_item_id, revision, product_id),
 KEY shopping_demand_list (list_id, id),
 CHECK (required > 0 AND purchased >= 0),
 CONSTRAINT shopping_demands_list_fk FOREIGN KEY (list_id) REFERENCES shopping_lists(id)
 ) ENGINE=InnoDB`)
	if err != nil {
		return fmt.Errorf("create shopping demands: %w", err)
	}
	_, err = v.client.ExecContext(
		ctx,
		"shopping.create_allocation_results",
		`CREATE TABLE IF NOT EXISTS shopping_allocation_results (
 revision BINARY(16) NOT NULL PRIMARY KEY,
 request_hash BINARY(32) NOT NULL
 ) ENGINE=InnoDB`,
	)
	if err != nil {
		return fmt.Errorf("create shopping allocation results: %w", err)
	}
	return nil
}

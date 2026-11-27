package migrations

import (
	"context"
	"fmt"

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

func Version1795794016(client mysql.ClientContext) migrator.Migration {
	return &version1795794016{
		client: client,
	}
}

type version1795794016 struct {
	client mysql.ClientContext
}

func (v *version1795794016) Version() int64 {
	return 1795794016
}

func (v *version1795794016) Description() string {
	return "Track cancelled demand sources"
}

func (v *version1795794016) Up(ctx context.Context) error {
	_, err := v.client.ExecContext(
		ctx,
		"shopping.create_source_states",
		`CREATE TABLE IF NOT EXISTS shopping_source_states (
 revision BINARY(16) NOT NULL PRIMARY KEY,
 list_id BINARY(16) NOT NULL, meal_plan_id BINARY(16) NOT NULL, meal_item_id BINARY(16) NOT NULL,
 cancelled BOOLEAN NOT NULL DEFAULT FALSE,
 CONSTRAINT shopping_source_states_list_fk FOREIGN KEY (list_id) REFERENCES shopping_lists(id)
 ) ENGINE=InnoDB`,
	)
	if err != nil {
		return fmt.Errorf("create shopping source states: %w", err)
	}
	return nil
}

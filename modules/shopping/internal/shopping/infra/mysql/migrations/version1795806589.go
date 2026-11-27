package migrations

import (
	"context"
	"fmt"

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

func Version1795806589(client mysql.ClientContext) migrator.Migration {
	return &version1795806589{
		client: client,
	}
}

type version1795806589 struct {
	client mysql.ClientContext
}

func (v *version1795806589) Version() int64 {
	return 1795806589
}

func (v *version1795806589) Description() string {
	return "Create manual demands and purchases"
}

func (v *version1795806589) Up(ctx context.Context) error {
	_, err := v.client.ExecContext(ctx, "shopping.add_manual_demands_and_purchases", `
CREATE TABLE IF NOT EXISTS shopping_purchases (
 id BINARY(16) NOT NULL PRIMARY KEY,
 list_id BINARY(16) NOT NULL,
 demand_id BINARY(16) NOT NULL,
 product_id BINARY(16) NOT NULL,
 inventory_id BINARY(16) NOT NULL,
 quantity BIGINT NOT NULL,
 confirmed BOOLEAN NOT NULL DEFAULT TRUE,
 KEY shopping_purchases_demand (demand_id, id),
 CONSTRAINT shopping_purchases_list_fk FOREIGN KEY (list_id) REFERENCES shopping_lists(id),
 CONSTRAINT shopping_purchases_demand_fk FOREIGN KEY (demand_id) REFERENCES shopping_demands(id),
 CHECK (quantity > 0)
) ENGINE=InnoDB`)
	if err != nil {
		return fmt.Errorf("create shopping purchases: %w", err)
	}
	return nil
}

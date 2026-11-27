package migrations

import (
	"context"
	"fmt"

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

func Version1795786855(client mysql.ClientContext) migrator.Migration {
	return &version1795786855{
		client: client,
	}
}

type version1795786855 struct {
	client mysql.ClientContext
}

func (v *version1795786855) Version() int64 {
	return 1795786855
}

func (v *version1795786855) Description() string {
	return "Create shopping lists"
}

func (v *version1795786855) Up(ctx context.Context) error {
	_, err := v.client.ExecContext(ctx, "shopping.create_lists", `CREATE TABLE IF NOT EXISTS shopping_lists (
 id BINARY(16) NOT NULL PRIMARY KEY,
 owner_id BINARY(16) NOT NULL,
 name VARCHAR(255) NOT NULL,
 KEY shopping_lists_owner (owner_id, id)
 ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	if err != nil {
		return fmt.Errorf("create shopping lists: %w", err)
	}
	return nil
}

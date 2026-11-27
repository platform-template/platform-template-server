package migrations

import (
	"context"
	"fmt"

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

func Version1795787381(client mysql.ClientContext) migrator.Migration {
	return &version1795787381{
		client: client,
	}
}

type version1795787381 struct {
	client mysql.ClientContext
}

func (v *version1795787381) Version() int64 {
	return 1795787381
}

func (v *version1795787381) Description() string {
	return "Create partial allocation operations"
}

func (v *version1795787381) Up(ctx context.Context) error {
	_, err := v.client.ExecContext(ctx, "allocation.create_schema", `CREATE TABLE IF NOT EXISTS allocation_operations (
 id BINARY(16) NOT NULL PRIMARY KEY,
 request_hash BINARY(32) NOT NULL,
 result JSON NULL
 ) ENGINE=InnoDB`)
	if err != nil {
		return fmt.Errorf("create allocation operations: %w", err)
	}
	return nil
}

package migrations

import (
	"context"
	"fmt"

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

func Version1795802139(client mysql.ClientContext) migrator.Migration {
	return &version1795802139{
		client: client,
	}
}

type version1795802139 struct {
	client mysql.ClientContext
}

func (v *version1795802139) Version() int64 {
	return 1795802139
}

func (v *version1795802139) Description() string {
	return "Bind allocation ownership"
}

func (v *version1795802139) Up(ctx context.Context) error {
	_, err := v.client.ExecContext(
		ctx,
		"allocation.create_identity",
		`CREATE TABLE IF NOT EXISTS allocation_identities (
 revision BINARY(16) NOT NULL PRIMARY KEY,
 identity_hash BINARY(32) NOT NULL
 ) ENGINE=InnoDB`,
	)
	if err != nil {
		return fmt.Errorf("create allocation identities: %w", err)
	}
	return nil
}

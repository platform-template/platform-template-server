package migrations

import (
	"context"

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	"github.com/distributed-programming-2026/lib/transaction/xa"
)

func Version1794380841(client mysql.ClientContext) migrator.Migration {
	return &version1794380841{
		client: client,
	}
}

type version1794380841 struct {
	client mysql.ClientContext
}

func (v *version1794380841) Version() int64 {
	return 1794380841
}

func (v *version1794380841) Description() string {
	return "Create XA abort barriers table"
}

func (v *version1794380841) Up(ctx context.Context) error {
	_, err := v.client.ExecContext(
		ctx,
		"xa.create_abort_barriers",
		xa.AbortBarrierMigrationSQL,
	)
	return err
}

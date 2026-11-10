package migrations

import (
	"context"

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	"github.com/distributed-programming-2026/lib/transaction/xa"
)

func Version1794345363(client mysql.ClientContext) migrator.Migration {
	return &version1794345363{
		client: client,
	}
}

type version1794345363 struct {
	client mysql.ClientContext
}

func (v *version1794345363) Version() int64 {
	return 1794345363
}

func (v *version1794345363) Description() string {
	return "Create XA branch receipts table"
}

func (v *version1794345363) Up(ctx context.Context) error {
	_, err := v.client.ExecContext(
		ctx,
		"xa.create_branch_receipts",
		xa.ReceiptMigrationSQL,
	)
	return err
}

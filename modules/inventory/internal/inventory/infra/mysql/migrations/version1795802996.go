package migrations

import (
	"context"
	"fmt"

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

func Version1795802996(client mysql.ClientContext) migrator.Migration {
	return &version1795802996{
		client: client,
	}
}

type version1795802996 struct {
	client mysql.ClientContext
}

func (v *version1795802996) Version() int64 {
	return 1795802996
}

func (v *version1795802996) Description() string {
	return "Consume meal allocation"
}

func (v *version1795802996) Up(ctx context.Context) error {
	var columns int
	err := v.client.QueryRowContext(ctx, "allocation.check_consumed", `SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'allocation_operations' AND column_name = 'consumed'`).
		Scan(&columns)
	if err != nil {
		return fmt.Errorf("check allocation consumed column: %w", err)
	}
	if columns != 0 {
		return nil
	}
	if _, err := v.client.ExecContext(
		ctx,
		"allocation.add_consumed",
		`ALTER TABLE allocation_operations ADD COLUMN consumed BOOLEAN NOT NULL DEFAULT FALSE`,
	); err != nil {
		return fmt.Errorf("add allocation consumed column: %w", err)
	}
	return nil
}

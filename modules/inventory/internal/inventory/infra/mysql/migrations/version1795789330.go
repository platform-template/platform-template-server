package migrations

import (
	"context"
	"fmt"

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

func Version1795789330(client mysql.ClientContext) migrator.Migration {
	return &version1795789330{
		client: client,
	}
}

type version1795789330 struct {
	client mysql.ClientContext
}

func (v *version1795789330) Version() int64 {
	return 1795789330
}

func (v *version1795789330) Description() string {
	return "Support allocation cancellation"
}

func (v *version1795789330) Up(ctx context.Context) error {
	if _, err := v.client.ExecContext(
		ctx,
		"allocation.allow_marker",
		`ALTER TABLE allocation_operations MODIFY request_hash BINARY(32) NULL`,
	); err != nil {
		return fmt.Errorf("allow cancellation markers: %w", err)
	}
	var columns int
	err := v.client.QueryRowContext(ctx, "allocation.check_status", `SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'allocation_operations' AND column_name = 'cancelled'`).
		Scan(&columns)
	if err != nil {
		return fmt.Errorf("check allocation status column: %w", err)
	}
	if columns == 0 {
		if _, err := v.client.ExecContext(
			ctx,
			"allocation.add_status",
			`ALTER TABLE allocation_operations ADD COLUMN cancelled BOOLEAN NOT NULL DEFAULT FALSE`,
		); err != nil {
			return fmt.Errorf("add allocation status: %w", err)
		}
	}
	return nil
}

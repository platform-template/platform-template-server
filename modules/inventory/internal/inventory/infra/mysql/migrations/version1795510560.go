package migrations

import (
	"context"

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

func Version1795510560(client mysql.ClientContext) migrator.Migration {
	return &version1795510560{
		client: client,
	}
}

type version1795510560 struct {
	client mysql.ClientContext
}

func (v *version1795510560) Version() int64 {
	return 1795510560
}

func (v *version1795510560) Description() string {
	return "Allow consumed reservations"
}

func (v *version1795510560) Up(ctx context.Context) error {
	_, err := v.client.ExecContext(ctx, "inventory.reservation_consumed", `ALTER TABLE reservations
DROP CHECK reservations_chk_1,
ADD CONSTRAINT reservations_status_check CHECK (status IN ('active', 'cancelled', 'consumed'))`)
	return err
}

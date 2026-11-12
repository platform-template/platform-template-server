package migrations

import (
	"context"

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

func Version1794456568(client mysql.ClientContext) migrator.Migration {
	return &version1794456568{
		client: client,
	}
}

type version1794456568 struct {
	client mysql.ClientContext
}

func (v *version1794456568) Version() int64 {
	return 1794456568
}

func (v *version1794456568) Description() string {
	return "Add registration transaction leases"
}

func (v *version1794456568) Up(ctx context.Context) error {
	_, err := v.client.ExecContext(
		ctx,
		"registration.add_leases",
		`
ALTER TABLE registration_transactions
    ADD COLUMN lease_owner CHAR(36)
        CHARACTER SET ascii COLLATE ascii_bin NULL,
    ADD COLUMN lease_until DATETIME(6) NULL,
    ADD COLUMN lease_epoch BIGINT UNSIGNED NOT NULL DEFAULT 0,
    ADD INDEX idx_registration_lease (state, lease_until)
`,
	)
	return err
}

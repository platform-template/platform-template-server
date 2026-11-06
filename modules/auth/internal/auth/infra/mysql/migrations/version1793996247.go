package migrations

import (
	"context"

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

func Version1793996247(client mysql.ClientContext) migrator.Migration {
	return &version1793996247{
		client: client,
	}
}

type version1793996247 struct {
	client mysql.ClientContext
}

func (v *version1793996247) Version() int64 {
	return 1793996247
}

func (v *version1793996247) Description() string {
	return "Create registration transactions table"
}

func (v *version1793996247) Up(ctx context.Context) error {
	_, err := v.client.ExecContext(
		ctx,
		"registration.create_transactions",
		`
CREATE TABLE IF NOT EXISTS registration_transactions (
    transaction_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL PRIMARY KEY,
    user_id CHAR(36) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
    state VARCHAR(32) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,

    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
        ON UPDATE CURRENT_TIMESTAMP(6),

    INDEX idx_registration_state (state)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
`,
	)
	return err
}

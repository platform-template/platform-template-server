package migrations

import (
	"context"

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

func Version1791225168(client mysql.ClientContext) migrator.Migration {
	return &version1791225168{client: client}
}

type version1791225168 struct {
	client mysql.ClientContext
}

func (v version1791225168) Version() int64 {
	return 1791225168
}

func (v version1791225168) Description() string {
	return "Create sessions table"
}

func (v version1791225168) Up(ctx context.Context) error {
	_, err := v.client.ExecContext(ctx, "create-sessions-table", `
CREATE TABLE IF NOT EXISTS sessions (
    id CHAR(36) CHARACTER SET ascii NOT NULL PRIMARY KEY,
    user_id CHAR(36) CHARACTER SET ascii NOT NULL,
    token_hash CHAR(64) CHARACTER SET ascii NOT NULL UNIQUE,
    expires_at DATETIME(6) NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    INDEX idx_sessions_user_id (user_id)
) ENGINE=InnoDB CHARACTER SET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
`)
	return err
}

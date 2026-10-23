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
	return "Create users table"
}

func (v version1791225168) Up(ctx context.Context) error {
	_, err := v.client.ExecContext(ctx, "create-users-table", `
CREATE TABLE IF NOT EXISTS users (
    id CHAR(36) CHARACTER SET ascii NOT NULL PRIMARY KEY,
    first_name VARCHAR(255) NOT NULL,
    last_name VARCHAR(255) NOT NULL,
    middle_name VARCHAR(255) NULL,
    login VARCHAR(255) NOT NULL UNIQUE,
    email VARCHAR(254) NOT NULL
) ENGINE=InnoDB CHARACTER SET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
`)
	return err
}

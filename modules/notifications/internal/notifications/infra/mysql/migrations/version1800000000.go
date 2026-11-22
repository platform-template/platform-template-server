package migrations

import (
	"context"

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

func Version1800000000(client mysql.ClientContext) migrator.Migration {
	return &version1800000000{client: client}
}

type version1800000000 struct{ client mysql.ClientContext }

func (version1800000000) Version() int64      { return 1800000000 }
func (version1800000000) Description() string { return "Create notifications" }
func (m version1800000000) Up(ctx context.Context) error {
	_, err := m.client.ExecContext(ctx, "notifications.create_table", `CREATE TABLE notifications (
id BINARY(16) NOT NULL PRIMARY KEY,
user_id BINARY(16) NOT NULL,
kind VARCHAR(128) NOT NULL,
title VARCHAR(255) NOT NULL,
body TEXT NOT NULL,
created_at DATETIME(6) NOT NULL,
read_at DATETIME(6) NULL,
idempotency_key VARCHAR(255) CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_bin NULL,
UNIQUE KEY notifications_idempotency (user_id, idempotency_key),
KEY notifications_user_created (user_id, created_at DESC, id DESC),
KEY notifications_user_unread (user_id, read_at, created_at DESC, id DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	return err
}

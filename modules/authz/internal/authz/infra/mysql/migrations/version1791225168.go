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
	return "Create authorization tables"
}

func (v version1791225168) Up(ctx context.Context) error {
	_, err := v.client.ExecContext(ctx, "authz.create_module_grants_table", `
CREATE TABLE IF NOT EXISTS module_grants (
    user_id CHAR(36) CHARACTER SET ascii NOT NULL,
    module_key VARCHAR(255) CHARACTER SET ascii NOT NULL,
    granted_by CHAR(36) CHARACTER SET ascii NULL,
    granted_at DATETIME(6) NOT NULL,
    PRIMARY KEY (user_id, module_key),
    INDEX idx_module_grants_user_id (user_id)
) ENGINE=InnoDB CHARACTER SET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
`)
	if err != nil {
		return err
	}
	_, err = v.client.ExecContext(ctx, "authz.create_system_roles_table", `
CREATE TABLE IF NOT EXISTS system_roles (
    user_id CHAR(36) CHARACTER SET ascii NOT NULL PRIMARY KEY,
    role VARCHAR(32) CHARACTER SET ascii NOT NULL,
    CONSTRAINT chk_system_roles_role CHECK (role = 'superuser')
) ENGINE=InnoDB CHARACTER SET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
`)
	return err
}

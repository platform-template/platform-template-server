package migrations

import (
	"context"

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

func Version1795382668(client mysql.ClientContext) migrator.Migration {
	return &version1795382668{
		client: client,
	}
}

type version1795382668 struct {
	client mysql.ClientContext
}

func (v *version1795382668) Version() int64 {
	return 1795382668
}

func (v *version1795382668) Description() string {
	return "Create audit entries"
}

func (v *version1795382668) Up(ctx context.Context) error {
	_, err := v.client.ExecContext(ctx, "audit.create_entries", `
CREATE TABLE audit_entries (
    event_id VARCHAR(255) COLLATE utf8mb4_0900_bin NOT NULL PRIMARY KEY,
    actor_id BINARY(16) NULL,
    module_key VARCHAR(128) COLLATE utf8mb4_0900_bin NOT NULL,
    action VARCHAR(128) COLLATE utf8mb4_0900_bin NOT NULL,
    target_type VARCHAR(128) COLLATE utf8mb4_0900_bin NOT NULL,
    target_id VARCHAR(255) COLLATE utf8mb4_0900_bin NOT NULL,
    correlation_id VARCHAR(255) COLLATE utf8mb4_0900_bin NOT NULL,
    metadata MEDIUMTEXT NOT NULL,
    occurred_at DATETIME(6) NOT NULL,
    recorded_at DATETIME(6) NOT NULL,
    CONSTRAINT audit_metadata_valid CHECK (JSON_VALID(metadata) AND OCTET_LENGTH(metadata) <= 16384),
    INDEX audit_occurred (occurred_at DESC, event_id DESC),
    INDEX audit_actor (actor_id, occurred_at DESC, event_id DESC),
    INDEX audit_module (module_key, occurred_at DESC, event_id DESC),
    INDEX audit_action (action, occurred_at DESC, event_id DESC),
    INDEX audit_target (target_type, target_id, occurred_at DESC, event_id DESC)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_0900_bin`)
	return err
}

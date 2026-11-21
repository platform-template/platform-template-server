package migrations

import (
	"context"

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

func Version1795000000(client mysql.ClientContext) migrator.Migration {
	return &version1795000000{client: client}
}

type version1795000000 struct {
	client mysql.ClientContext
}

func (v version1795000000) Version() int64 { return 1795000000 }

func (v version1795000000) Description() string { return "Create initial superuser lock" }

func (v version1795000000) Up(ctx context.Context) error {
	_, err := v.client.ExecContext(
		ctx,
		"authz.create_initial_superuser_lock",
		`
CREATE TABLE IF NOT EXISTS initial_superuser_lock (
    id TINYINT NOT NULL PRIMARY KEY
) ENGINE=InnoDB CHARACTER SET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
`,
	)
	if err != nil {
		return err
	}
	_, err = v.client.ExecContext(
		ctx,
		"authz.seed_initial_superuser_lock",
		`
INSERT IGNORE INTO initial_superuser_lock (id) VALUES (1);
`,
	)
	return err
}

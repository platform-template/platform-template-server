package migrations

import (
	"context"

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

func Version1792754706(client mysql.ClientContext) migrator.Migration {
	return &version1792754706{client: client}
}

type version1792754706 struct {
	client mysql.ClientContext
}

func (v version1792754706) Version() int64 {
	return 1792754706
}

func (v version1792754706) Description() string {
	return "Make user middle name optional"
}

func (v version1792754706) Up(ctx context.Context) error {
	_, err := v.client.ExecContext(ctx, "make-middle-name-optional", `
ALTER TABLE users MODIFY middle_name VARCHAR(255) NULL;
`)
	return err
}

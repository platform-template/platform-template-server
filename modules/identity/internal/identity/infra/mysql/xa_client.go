package mysql

import (
	"context"
	"database/sql"

	"github.com/distributed-programming-2026/lib/transaction/xa"

	"identity/internal/identity/infra/mysql/repository"
)

type xaClient struct {
	executor xa.Executor
}

var _ repository.Client = (*xaClient)(nil)

func NewXAClient(executor xa.Executor) repository.Client {
	return &xaClient{executor: executor}
}

func (c *xaClient) ExecContext(
	ctx context.Context,
	_ string,
	query string,
	args ...any,
) (sql.Result, error) {
	return c.executor.ExecContext(ctx, query, args...)
}

func (c *xaClient) QueryRowContext(
	ctx context.Context,
	_ string,
	query string,
	args ...any,
) *sql.Row {
	return c.executor.QueryRowContext(ctx, query, args...)
}

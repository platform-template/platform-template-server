package main

import (
	"context"
	"log/slog"

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"

	"authz/internal/authz/infra/mysql/migrations"
)

func migrateDatabase(
	ctx context.Context,
	logger *slog.Logger,
	client mysql.TransactionalClient,
) error {
	runner, err := migrator.New(client, logger)
	if err != nil {
		return err
	}
	return runner.Migrate(ctx, appID, migrations.Database(client)...)
}

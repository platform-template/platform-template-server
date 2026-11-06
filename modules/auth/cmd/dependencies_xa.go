package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/distributed-programming-2026/lib/transaction/xa"

	"auth/internal/auth/config"
)

func openXAResource(
	ctx context.Context,
	conf config.Env,
) (*xa.MySQL, *sql.DB, error) {
	db, err := sql.Open("mysql", mysqlDSN(conf))
	if err != nil {
		return nil, nil, fmt.Errorf("open XA database: %w", err)
	}

	closeWithError := func(err error) (*xa.MySQL, *sql.DB, error) {
		return nil, nil, errors.Join(err, db.Close())
	}

	db.SetMaxOpenConns(conf.MySQL.MaxConnections)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(conf.MySQL.ConnectionMaxLifeTime)
	db.SetConnMaxIdleTime(conf.MySQL.ConnectionMaxIdleTime)

	if err := db.PingContext(ctx); err != nil {
		return closeWithError(fmt.Errorf("ping XA database: %w", err))
	}

	var detached int

	err = db.QueryRowContext(
		ctx,
		"SELECT @@SESSION.xa_detach_on_prepare",
	).Scan(&detached)
	if err != nil {
		return closeWithError(fmt.Errorf("check XA configuration: %w", err))
	}

	if detached != 1 {
		return closeWithError(
			errors.New("XA requires xa_detach_on_prepare=ON"),
		)
	}

	resource, err := xa.NewMySQL(db)
	if err != nil {
		return closeWithError(fmt.Errorf("create XA resource: %w", err))
	}

	return resource, db, nil
}

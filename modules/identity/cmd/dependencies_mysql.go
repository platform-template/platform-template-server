package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	mysqldriver "github.com/go-sql-driver/mysql"

	"github.com/distributed-programming-2026/lib/transaction/xa"

	"identity/internal/identity/config"
	identitymysql "identity/internal/identity/infra/mysql"
)

type database struct {
	connector mysql.Connector
	unit      uow.UnitOfWorkWithRepositoryProvider[*identitymysql.RepositoryProvider]
}

func mysqlDSN(conf config.Env) string {
	dsn := mysqldriver.NewConfig()
	dsn.Net = "tcp"
	dsn.Addr = conf.MySQL.Address
	dsn.DBName = conf.MySQL.Database
	dsn.User = conf.MySQL.User
	dsn.Passwd = conf.MySQL.Password
	dsn.ParseTime = true
	dsn.Timeout = conf.MySQL.ConnectTimeout
	dsn.ReadTimeout = conf.MySQL.ReadTimeout
	dsn.WriteTimeout = conf.MySQL.WriteTimeout
	dsn.Params = map[string]string{
		"charset": "utf8mb4",
	}

	return dsn.FormatDSN()
}

func openDatabase(
	ctx context.Context,
	logger *slog.Logger,
	conf config.Env,
) (_ *database, err error) {
	connector := mysql.NewConnector()

	err = connector.Open(mysqlDSN(conf), mysql.Config{
		MaxConnections:        conf.MySQL.MaxConnections,
		ConnectionMaxLifeTime: conf.MySQL.ConnectionMaxLifeTime,
		ConnectionMaxIdleTime: conf.MySQL.ConnectionMaxIdleTime,
	})
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			err = errors.Join(err, connector.Close())
		}
	}()

	client := connector.TransactionalClient()

	if err := migrateDatabase(ctx, logger, client); err != nil {
		return nil, err
	}

	unit := uow.NewUnitOfWork(
		mysql.NewConnectionPool(client),
		identitymysql.NewRepositoryProvider,
	)

	return &database{
		connector: connector,
		unit:      unit,
	}, nil
}

func openXAResource(
	ctx context.Context,
	conf config.Env,
) (_ *xa.MySQL, db *sql.DB, err error) {
	db, err = sql.Open("mysql", mysqlDSN(conf))
	if err != nil {
		return nil, nil, fmt.Errorf("open XA database: %w", err)
	}

	defer func() {
		if err != nil {
			err = errors.Join(err, db.Close())
		}
	}()

	db.SetMaxOpenConns(conf.MySQL.MaxConnections)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(conf.MySQL.ConnectionMaxLifeTime)
	db.SetConnMaxIdleTime(conf.MySQL.ConnectionMaxIdleTime)

	if err := db.PingContext(ctx); err != nil {
		return nil, nil, fmt.Errorf("ping XA database: %w", err)
	}

	var detached int
	err = db.QueryRowContext(
		ctx,
		"SELECT @@SESSION.xa_detach_on_prepare",
	).Scan(&detached)
	if err != nil {
		return nil, nil, fmt.Errorf("check XA configuration: %w", err)
	}
	if detached != 1 {
		return nil, nil, errors.New("XA requires xa_detach_on_prepare=ON")
	}

	resource, err := xa.NewMySQL(db)
	if err != nil {
		return nil, nil, fmt.Errorf("create XA resource: %w", err)
	}

	return resource, db, nil
}

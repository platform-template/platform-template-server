package main

import (
	"context"
	"errors"
	"os"

	"github.com/distributed-programming-2026/go-sdk/pkg/logging"
	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	"github.com/kelseyhightower/envconfig"
	"github.com/urfave/cli/v3"

	"authz/internal/authz/app"
	"authz/internal/authz/config"
	authzmysql "authz/internal/authz/infra/mysql"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

const appID = "authz"

func main() {
	if err := command().Run(context.Background(), os.Args); err != nil {
		os.Exit(1)
	}
}

func command() *cli.Command {
	var conf config.Env
	logger := logging.New(logging.Config{AppID: appID})
	return &cli.Command{
		Name:           appID,
		DefaultCommand: "migrate",
		Before: func(ctx context.Context, _ *cli.Command) (context.Context, error) {
			if err := envconfig.Process(appID, &conf); err != nil {
				return ctx, err
			}
			return ctx, conf.Validate()
		},
		Commands: []*cli.Command{{
			Name: "migrate",
			Action: func(ctx context.Context, _ *cli.Command) (err error) {
				connector, err := openDatabase(conf)
				if err != nil {
					return err
				}
				defer func() { err = errors.Join(err, connector.Close()) }()
				return migrateDatabase(
					ctx,
					logger,
					connector.TransactionalClient(),
				)
			},
		}, {
			Name: "bootstrap-superuser",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:     "user-id",
					Required: true,
				},
			},
			Action: func(ctx context.Context, c *cli.Command) (err error) {
				userID, err := uuid.Parse(c.String("user-id"))
				if err != nil || userID == uuid.Nil() {
					return errors.New("invalid user ID")
				}
				connector, err := openDatabase(conf)
				if err != nil {
					return err
				}
				defer func() { err = errors.Join(err, connector.Close()) }()
				if err = migrateDatabase(
					ctx,
					logger,
					connector.TransactionalClient(),
				); err != nil {
					return err
				}
				unit := uow.NewUnitOfWork(
					mysql.NewConnectionPool(
						connector.TransactionalClient(),
					),
					authzmysql.NewRepositoryProvider,
				)
				return app.NewService(unit).BootstrapSuperuser(ctx, userID)
			},
		}, serviceCmd(&conf, logger),
		},
	}
}

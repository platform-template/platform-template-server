package main

import (
	"context"
	"errors"
	"os"

	"github.com/distributed-programming-2026/go-sdk/pkg/logging"
	"github.com/kelseyhightower/envconfig"
	"github.com/urfave/cli/v3"

	"authz/internal/authz/config"
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
				return migrateDatabase(ctx, logger, connector.TransactionalClient())
			},
		}},
	}
}

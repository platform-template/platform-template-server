package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"
	grpcclient "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	inventoryapi "inventory/api/server/internalapi"
	"recipes/internal/recipes/app"
	"recipes/internal/recipes/config"
	eventinfra "recipes/internal/recipes/infra/event"
	"recipes/internal/recipes/infra/grpc/catalog"
)

func (a *application) serviceCmd() *cli.Command {
	return &cli.Command{
		Name: "service",
		Action: func(ctx context.Context, _ *cli.Command) error {
			return service(ctx, a.logger, a.conf)
		},
	}
}

func service(ctx context.Context, logger *slog.Logger, conf config.Env) (err error) {
	db, err := openDatabase(ctx, logger, conf)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, db.connector.Close())
	}()

	connection := newConnection(logger, conf)
	defer func() {
		err = errors.Join(err, connection.Stop())
	}()

	dispatcher, relay, err := newDispatcher(logger, conf, db, connection)
	if err != nil {
		return err
	}

	err = connection.Start()
	if err != nil {
		return err
	}

	dispatcherFactory := eventinfra.NewDispatcherFactory(appID, dispatcher)
	inventoryConn, err := grpcclient.NewClient(
		conf.Inventory.Address,
		grpcclient.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("create inventory client: %w", err)
	}
	defer func() { err = errors.Join(err, inventoryConn.Close()) }()
	svc := app.NewService(
		db.unit,
		dispatcherFactory,
		catalog.NewClient(inventoryapi.NewInventoryServiceClient(inventoryConn)),
	)
	httpServer, err := newHTTPServer(logger, conf, svc)
	if err != nil {
		return err
	}

	grpcServer, err := newGRPCServer(logger, conf, svc)
	if err != nil {
		return err
	}

	errGroup, egCtx := errgroup.WithContext(ctx)
	errGroup.Go(func() error {
		return httpServer.Serve(egCtx)
	})
	errGroup.Go(func() error {
		return grpcServer.Serve(egCtx)
	})
	errGroup.Go(func() error {
		return runRelay(egCtx, logger, relay, conf.Relay)
	})
	return errGroup.Wait()
}

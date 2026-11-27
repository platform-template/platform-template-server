package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	sessionapi "session/api/server/internalapi"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"

	"shopping/internal/shopping/app"
	"shopping/internal/shopping/config"
	eventinfra "shopping/internal/shopping/infra/event"
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
	svc := app.NewService(db.unit, dispatcherFactory)
	sessionConn, err := grpc.NewClient(conf.Session.Address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("create session client: %w", err)
	}
	defer func() { err = errors.Join(err, sessionConn.Close()) }()
	httpServer, err := newHTTPServer(logger, conf, svc, sessionapi.NewSessionServiceClient(sessionConn))
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

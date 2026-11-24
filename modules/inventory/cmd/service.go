package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	authzapi "authz/api/server/internalapi"
	sessionapi "session/api/server/internalapi"

	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"
	grpcclient "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"inventory/internal/inventory/app"
	"inventory/internal/inventory/config"
	eventinfra "inventory/internal/inventory/infra/event"
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
	sessionAddress := conf.Session.Address
	if sessionAddress == "" {
		sessionAddress = "session-service:8081"
	}
	authzAddress := conf.Authz.Address
	if authzAddress == "" {
		authzAddress = "authz-service:8081"
	}
	sessionConn, err := grpcclient.NewClient(
		sessionAddress,
		grpcclient.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("create session client: %w", err)
	}
	defer func() { err = errors.Join(err, sessionConn.Close()) }()
	authzConn, err := grpcclient.NewClient(
		authzAddress,
		grpcclient.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("create authz client: %w", err)
	}
	defer func() { err = errors.Join(err, authzConn.Close()) }()
	httpServer, err := newHTTPServer(logger, conf, svc,
		sessionapi.NewSessionServiceClient(sessionConn), authzapi.NewAuthzServiceClient(authzConn))
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

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"

	"auth/internal/auth/app"
	"auth/internal/auth/config"
	"auth/internal/auth/infra/mysql/registrationjournal"
	"auth/internal/auth/infra/password"
	"auth/internal/auth/infra/recovery"
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

	_, relay, err := newDispatcher(logger, conf, db, connection)
	if err != nil {
		return err
	}

	err = connection.Start()
	if err != nil {
		return err
	}

	identityClient, sessionClient, closeClients, err := newGRPCClients(conf)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, closeClients())
	}()

	registrationIdentity, ok := identityClient.(app.RegistrationIdentityClient)
	if !ok {
		return fmt.Errorf(
			"identity client %T does not support registration XA",
			identityClient,
		)
	}

	xaResource, xaDB, err := openXAResource(ctx, conf)
	if err != nil {
		return fmt.Errorf("open registration XA resource: %w", err)
	}
	defer func() {
		err = errors.Join(err, xaDB.Close())
	}()

	journal := registrationjournal.NewRegistrationJournal(xaDB)
	leases := registrationjournal.NewLeaseStore(xaDB)

	authParticipant := app.NewRegistrationParticipant(
		xaResource,
		password.NewArgon2Hasher(),
	)

	registrationCoordinator := app.NewRegistrationCoordinator(
		journal,
		leases,
		authParticipant,
		registrationIdentity,
	)

	recoveryWorker, err := recovery.NewWorker(
		registrationCoordinator,
		logger,
		recovery.Config{
			Interval:       10 * time.Second,
			AttemptTimeout: 2 * time.Minute,
		},
	)
	if err != nil {
		return fmt.Errorf("create recovery worker: %w", err)
	}

	svc := app.NewService(
		db.unit,
		password.NewArgon2Hasher(),
		identityClient,
		sessionClient,
		registrationCoordinator,
	)

	httpServer, err := newHTTPServer(logger, conf, svc)
	if err != nil {
		return err
	}

	errGroup, egCtx := errgroup.WithContext(ctx)
	errGroup.Go(func() error {
		return httpServer.Serve(egCtx)
	})
	errGroup.Go(func() error {
		return runRelay(egCtx, logger, relay, conf.Relay)
	})
	errGroup.Go(func() error {
		return recoveryWorker.Run(egCtx)
	})
	return errGroup.Wait()
}

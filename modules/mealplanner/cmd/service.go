package main

import (
	"context"
	"errors"
	"fmt"
	inventoryapi "inventory/api/server/internalapi"
	"log/slog"
	recipesapi "recipes/api/server/internalapi"
	sessionapi "session/api/server/internalapi"
	shoppingapi "shopping/api/server/internalapi"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"

	"mealplanner/internal/mealplanner/app"
	"mealplanner/internal/mealplanner/config"
	eventinfra "mealplanner/internal/mealplanner/infra/event"
	inventoryclient "mealplanner/internal/mealplanner/infra/grpc/inventory"
	recipesclient "mealplanner/internal/mealplanner/infra/grpc/recipes"
	shoppingclient "mealplanner/internal/mealplanner/infra/grpc/shopping"
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
	inventoryConn, err := grpc.NewClient(
		conf.Inventory.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("create inventory client: %w", err)
	}
	defer func() { err = errors.Join(err, inventoryConn.Close()) }()
	recipesConn, err := grpc.NewClient(conf.Recipes.Address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("create recipes client: %w", err)
	}
	defer func() { err = errors.Join(err, recipesConn.Close()) }()
	shoppingConn, err := grpc.NewClient(conf.Shopping.Address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("create shopping client: %w", err)
	}
	defer func() { err = errors.Join(err, shoppingConn.Close()) }()
	svc = app.NewPlanningService(svc, app.PlanningDependencies{
		Inventory: inventoryclient.NewClient(inventoryapi.NewInventoryServiceClient(inventoryConn)),
		Recipes:   recipesclient.NewClient(recipesapi.NewRecipesServiceClient(recipesConn)),
		Shopping:  shoppingclient.NewClient(shoppingapi.NewShoppingServiceClient(shoppingConn)),
	})
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

package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	interceptorgrpc "github.com/distributed-programming-2026/lib/gateway/interceptor"
	"github.com/urfave/cli/v3"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	internalapi "authz/api/server/internalapi"
	"authz/internal/authz/app"
	"authz/internal/authz/config"
	sessionclient "authz/internal/authz/infra/grpc/session"
	interceptorhandler "authz/internal/authz/infra/handlers/interceptor"
	internalhandler "authz/internal/authz/infra/handlers/internalapi"
	publichandler "authz/internal/authz/infra/handlers/public"
	authzmysql "authz/internal/authz/infra/mysql"

	grpcserverruntime "github.com/distributed-programming-2026/lib/runtime/grpc"
	runtimehttp "github.com/distributed-programming-2026/lib/runtime/http"
)

func serviceCmd(conf *config.Env, logger *slog.Logger) *cli.Command {
	return &cli.Command{
		Name: "service",
		Action: func(ctx context.Context, _ *cli.Command) error {
			return serve(ctx, *conf, logger)
		},
	}
}

func serve(
	ctx context.Context,
	conf config.Env,
	logger *slog.Logger,
) (err error) {
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
	connection, err := grpc.NewClient(
		conf.Session.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return fmt.Errorf("connect session: %w", err)
	}
	defer func() { err = errors.Join(err, connection.Close()) }()
	unit := uow.NewUnitOfWork(
		mysql.NewConnectionPool(connector.TransactionalClient()),
		authzmysql.NewRepositoryProvider,
	)
	svc := app.NewService(unit)
	httpRegister, err := publichandler.Register(
		svc,
		sessionclient.NewClient(connection),
	)
	if err != nil {
		return err
	}
	httpServer, err := runtimehttp.NewServer(
		runtimehttp.Config{
			Address:      conf.HTTP.Address,
			GraceTimeout: conf.App.GraceTimeout,
		},
		httpRegister,
	)
	if err != nil {
		return err
	}
	server, err := grpcserverruntime.NewServer(
		grpcserverruntime.Config{
			Address:      conf.GRPC.Address,
			GraceTimeout: conf.App.GraceTimeout,
		},
		func(server *grpc.Server) error {
			interceptorgrpc.RegisterInterceptorServer(
				server,
				interceptorhandler.NewHandler(
					sessionclient.NewClient(connection),
					svc,
				),
			)
			internalapi.RegisterAuthzServiceServer(
				server,
				internalhandler.NewHandler(
					svc,
					sessionclient.NewClient(connection),
					logger,
				),
			)
			return nil
		},
	)
	if err != nil {
		return err
	}
	group, groupCtx := errgroup.WithContext(ctx)
	group.Go(func() error { return server.Serve(groupCtx) })
	group.Go(func() error { return httpServer.Serve(groupCtx) })
	return group.Wait()
}

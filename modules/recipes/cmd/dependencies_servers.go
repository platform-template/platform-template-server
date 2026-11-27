package main

import (
	"log/slog"

	sessionapi "session/api/server/internalapi"

	gatewayapi "github.com/distributed-programming-2026/lib/gateway/interceptor"
	grpcserver "google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/distributed-programming-2026/lib/runtime/grpc"
	"github.com/distributed-programming-2026/lib/runtime/http"

	internalapi "recipes/api/server/internalapi"
	"recipes/internal/recipes/app"
	"recipes/internal/recipes/config"
	gatewayhandler "recipes/internal/recipes/infra/handlers/gateway"
	internalhandler "recipes/internal/recipes/infra/handlers/internalapi"
	publichandler "recipes/internal/recipes/infra/handlers/public"
)

func newHTTPServer(
	logger *slog.Logger,
	conf config.Env,
	svc *app.Service,
	sessions sessionapi.SessionServiceClient,
) (*http.Server, error) {
	register, err := publichandler.Register(svc, logger, sessions)
	if err != nil {
		return nil, err
	}

	return http.NewServer(http.Config{
		Address:      conf.HTTP.Address,
		GraceTimeout: conf.App.GraceTimeout,
	}, register)
}

func newGRPCServer(logger *slog.Logger, conf config.Env, svc *app.Service) (*grpc.Server, error) {
	return grpc.NewServer(grpc.Config{
		Address:      conf.GRPC.Address,
		GraceTimeout: conf.App.GraceTimeout,
	}, func(server *grpcserver.Server) error {
		internalapi.RegisterRecipesServiceServer(server, internalhandler.NewHandler(svc, logger))
		gatewayapi.RegisterInterceptorServer(server, gatewayhandler.NewHandler())
		reflection.Register(server)
		return nil
	})
}

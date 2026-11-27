package main

import (
	"log/slog"
	sessionapi "session/api/server/internalapi"

	gatewayapi "github.com/distributed-programming-2026/lib/gateway/interceptor"
	grpcserver "google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/distributed-programming-2026/lib/runtime/grpc"
	"github.com/distributed-programming-2026/lib/runtime/http"

	internalapi "shopping/api/server/internalapi"
	"shopping/internal/shopping/app"
	"shopping/internal/shopping/config"
	gatewayhandler "shopping/internal/shopping/infra/handlers/gateway"
	internalhandler "shopping/internal/shopping/infra/handlers/internalapi"
	publichandler "shopping/internal/shopping/infra/handlers/public"
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
		internalapi.RegisterShoppingServiceServer(server, internalhandler.NewHandler(svc, logger))
		gatewayapi.RegisterInterceptorServer(server, gatewayhandler.NewHandler())
		reflection.Register(server)
		return nil
	})
}

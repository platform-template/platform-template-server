package main

import (
	"log/slog"

	gatewayapi "github.com/distributed-programming-2026/lib/gateway/interceptor"
	grpcserver "google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/distributed-programming-2026/lib/runtime/grpc"
	"github.com/distributed-programming-2026/lib/runtime/http"

	internalapi "bff/api/server/internalapi"
	"bff/internal/bff/app"
	"bff/internal/bff/config"
	gatewayhandler "bff/internal/bff/infra/handlers/gateway"
	internalhandler "bff/internal/bff/infra/handlers/internalapi"
	publichandler "bff/internal/bff/infra/handlers/public"
)

func newHTTPServer(
	logger *slog.Logger,
	conf config.Env,
	svc *app.Service,
	bootstrap *app.BootstrapService,
) (*http.Server, error) {
	register, err := publichandler.Register(svc, bootstrap, logger)
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
		internalapi.RegisterBffServiceServer(server, internalhandler.NewHandler(svc, logger))
		gatewayapi.RegisterInterceptorServer(server, gatewayhandler.NewHandler())
		reflection.Register(server)
		return nil
	})
}

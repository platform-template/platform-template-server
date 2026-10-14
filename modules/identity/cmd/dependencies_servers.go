package main

import (
	"log/slog"

	grpcserver "google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/distributed-programming-2026/lib/runtime/grpc"
	"github.com/distributed-programming-2026/lib/runtime/http"

	internalapi "identity/api/server/internalapi"
	"identity/internal/identity/app"
	"identity/internal/identity/config"
	internalhandler "identity/internal/identity/infra/handlers/internalapi"
)

func newHTTPServer(_ *slog.Logger, conf config.Env, _ *app.Service) (*http.Server, error) {
	return http.NewServer(http.Config{
		Address:      conf.HTTP.Address,
		GraceTimeout: conf.App.GraceTimeout,
	})
}

func newGRPCServer(logger *slog.Logger, conf config.Env, svc *app.Service) (*grpc.Server, error) {
	return grpc.NewServer(grpc.Config{
		Address:      conf.GRPC.Address,
		GraceTimeout: conf.App.GraceTimeout,
	}, func(server *grpcserver.Server) error {
		internalapi.RegisterIdentityServiceServer(server, internalhandler.NewHandler(svc, logger))
		reflection.Register(server)
		return nil
	})
}

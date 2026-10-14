package main

import (
	"errors"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"auth/internal/auth/app"
	"auth/internal/auth/config"
	identitygrpc "auth/internal/auth/infra/grpc/identity"
	sessiongrpc "auth/internal/auth/infra/grpc/session"
)

func newGRPCClients(conf config.Env) (app.IdentityClient, app.SessionClient, func() error, error) {
	identityConn, err := grpc.NewClient(conf.Identity.Address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, nil, fmt.Errorf("connect identity: %w", err)
	}

	sessionConn, err := grpc.NewClient(conf.Session.Address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		_ = identityConn.Close()
		return nil, nil, nil, fmt.Errorf("connect session: %w", err)
	}

	closeClients := func() error {
		return errors.Join(identityConn.Close(), sessionConn.Close())
	}

	return identitygrpc.NewClient(identityConn), sessiongrpc.NewClient(sessionConn), closeClients, nil
}

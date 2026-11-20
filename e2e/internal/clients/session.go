package clients

import (
	"context"

	"e2e/internal/testenv"
	sessionapi "session/api/server/internalapi"
)

func Session(ctx context.Context, env *testenv.Environment) (sessionapi.SessionServiceClient, error) {
	conn, err := env.GRPC(ctx, "session")
	if err != nil {
		return nil, err
	}
	return sessionapi.NewSessionServiceClient(conn), nil
}

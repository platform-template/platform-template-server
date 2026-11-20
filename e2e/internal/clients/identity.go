package clients

import (
	"context"

	"e2e/internal/testenv"
	identityapi "identity/api/server/internalapi"
)

func Identity(ctx context.Context, env *testenv.Environment) (identityapi.IdentityServiceClient, error) {
	conn, err := env.GRPC(ctx, "identity")
	if err != nil {
		return nil, err
	}
	return identityapi.NewIdentityServiceClient(conn), nil
}

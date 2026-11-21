package clients

import (
	publicapi "auth/api/server/publicapi"
	"context"
	"e2e/internal/testenv"
)

func Auth(ctx context.Context, env *testenv.Environment) (*publicapi.ClientWithResponses, error) {
	address, err := env.GatewayAddress(ctx)
	if err != nil {
		return nil, err
	}
	return publicapi.NewClientWithResponses(address)
}

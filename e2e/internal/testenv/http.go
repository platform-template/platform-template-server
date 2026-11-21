package testenv

import "context"

func (e *Environment) HTTPAddress(ctx context.Context, service string) (string, error) {
	addr, err := e.Address(ctx, service, "8082/tcp")
	if err != nil {
		return "", err
	}
	return "http://" + addr, nil
}

func (e *Environment) GatewayAddress(ctx context.Context) (string, error) {
	addr, err := e.Address(ctx, "gateway", "8000/tcp")
	if err != nil {
		return "", err
	}
	return "http://" + addr, nil
}

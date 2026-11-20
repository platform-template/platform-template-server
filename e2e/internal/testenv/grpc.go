package testenv

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func (e *Environment) GRPC(ctx context.Context, service string) (*grpc.ClientConn, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil, fmt.Errorf("connect %s gRPC: environment is closed", service)
	}
	if conn := e.connections[service]; conn != nil {
		return conn, nil
	}
	addr, err := e.address(ctx, service, "8081/tcp")
	if err != nil {
		return nil, err
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("create %s gRPC client: %w", service, err)
	}
	e.connections[service] = conn
	return conn, nil
}

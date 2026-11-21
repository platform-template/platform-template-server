package testenv

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/compose"
	"google.golang.org/grpc"
)

type Environment struct {
	stack   *compose.DockerCompose
	project string

	mu          sync.Mutex
	closed      bool
	closeErr    error
	connections map[string]*grpc.ClientConn
}

func Start(ctx context.Context, composeFile string) (_ *Environment, err error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, fmt.Errorf("generate compose project name: %w", err)
	}
	project := fmt.Sprintf("e2e-%x", id)
	stack, err := compose.NewDockerComposeWith(
		compose.StackIdentifier(project),
		compose.WithStackFiles(composeFile),
	)
	if err != nil {
		return nil, fmt.Errorf("create compose stack: %w", err)
	}
	env := &Environment{
		stack:       stack,
		project:     project,
		connections: make(map[string]*grpc.ClientConn),
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, env.Close())
		}
	}()

	if err = env.start(ctx); err != nil {
		return nil, err
	}
	return env, nil
}

func (e *Environment) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return e.closeErr
	}
	e.closed = true

	var errs []error
	for name, conn := range e.connections {
		if err := conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close %s gRPC: %w", name, err))
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	logCtx, cancelLogs := context.WithTimeout(ctx, 10*time.Second)
	if err := e.reportFailedContainers(logCtx, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "E2E: could not collect failed container logs: %v\n", err)
	}
	cancelLogs()
	if err := e.stack.Down(ctx, compose.RemoveOrphans(true), compose.RemoveVolumes(true)); err != nil {
		errs = append(errs, fmt.Errorf("remove compose stack: %w", err))
	}
	if err := e.removeProjectVolumes(ctx); err != nil {
		errs = append(errs, fmt.Errorf("remove remaining test volumes: %w", err))
	}
	if err := e.stack.Close(); err != nil {
		errs = append(errs, fmt.Errorf("close compose client: %w", err))
	}
	e.closeErr = errors.Join(errs...)
	return e.closeErr
}

package testenv

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/testcontainers/testcontainers-go/modules/compose"
	"github.com/testcontainers/testcontainers-go/wait"
)

func (e *Environment) start(ctx context.Context) error {
	if err := e.stack.Up(ctx, compose.Wait(true)); err != nil {
		return fmt.Errorf("start compose stack: %w", err)
	}

	for _, name := range e.stack.Services() {
		container, err := e.stack.ServiceContainer(ctx, name)
		if err != nil {
			return fmt.Errorf("find compose service %q for readiness: %w", name, err)
		}
		info, err := container.Inspect(ctx)
		if err != nil {
			return fmt.Errorf("inspect compose service %q for readiness: %w", name, err)
		}
		if info.Config == nil {
			return fmt.Errorf("compose service %q has no container configuration", name)
		}
		port, configured := info.Config.Labels["e2e.http-health-port"]
		if !configured {
			continue
		}
		if port == "" {
			return fmt.Errorf("compose service %q has an empty HTTP readiness port", name)
		}
		strategy := wait.ForHTTP("/healthz").
			WithPort(port).
			WithStatusCodeMatcher(func(code int) bool { return code == http.StatusNoContent }).
			WithStartupTimeout(2 * time.Minute)
		if err := strategy.WaitUntilReady(ctx, container); err != nil {
			return fmt.Errorf("service %q HTTP readiness: %w", name, err)
		}
	}
	return nil
}

func (e *Environment) Address(ctx context.Context, name, port string) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return "", fmt.Errorf("resolve %s: environment is closed", name)
	}
	return e.address(ctx, name, port)
}

func (e *Environment) address(ctx context.Context, name, port string) (string, error) {
	container, err := e.stack.ServiceContainer(ctx, name)
	if err != nil {
		return "", fmt.Errorf("find compose service %q: %w", name, err)
	}
	host, err := container.Host(ctx)
	if err != nil {
		return "", fmt.Errorf("resolve host for %q: %w", name, err)
	}
	mapped, err := container.MappedPort(ctx, port)
	if err != nil {
		return "", fmt.Errorf("resolve %s port %s: %w", name, port, err)
	}
	return net.JoinHostPort(host, mapped.Port()), nil
}

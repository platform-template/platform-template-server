//go:build e2e

package e2e

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
)

var services = map[string]testcontainers.Container{}
var databases = map[string]*tcmysql.MySQLContainer{}

type environment struct {
	network *testcontainers.DockerNetwork
	closers []func() error
}

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) (result int) {
	env := &environment{}
	defer func() {
		if err := env.close(); err != nil {
			fmt.Fprintln(os.Stderr, "cleanup:", err)
			result = 1
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := env.prepare(ctx); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return m.Run()
}

func (e *environment) prepare(ctx context.Context) error {
	var err error
	e.network, err = network.New(ctx)
	if err != nil {
		return fmt.Errorf("create network: %w", err)
	}
	e.closers = append(e.closers, func() error {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return e.network.Remove(ctx)
	})

	for _, name := range []string{"auth", "identity", "session"} {
		if err := e.startDatabase(ctx, name); err != nil {
			return fmt.Errorf("%s mysql: %w", name, err)
		}
	}
	if err := e.startBroker(ctx); err != nil {
		return fmt.Errorf("rabbitmq: %w", err)
	}
	for _, name := range []string{"identity", "session", "auth"} {
		if err := e.startService(ctx, name); err != nil {
			return fmt.Errorf("%s service: %w", name, err)
		}
	}
	return nil
}

func (e *environment) startDatabase(ctx context.Context, name string) error {
	db, err := tcmysql.Run(ctx, "mysql:8.4",
		tcmysql.WithDatabase(name+"_test"),
		tcmysql.WithUsername(name),
		tcmysql.WithPassword("e2e-password"),
		network.WithNetwork([]string{name + "-mysql"}, e.network),
	)
	if db != nil {
		e.closers = append(e.closers, func() error {
			return testcontainers.TerminateContainer(db)
		})
	}
	if err != nil {
		return err
	}
	databases[name] = db
	return nil
}

func (e *environment) startBroker(ctx context.Context) error {
	_, err := e.startContainer(ctx, testcontainers.ContainerRequest{
		Image:    "rabbitmq:4.1-alpine",
		Networks: []string{e.network.Name},
		NetworkAliases: map[string][]string{
			e.network.Name: {"rabbitmq"},
		},
		Env: map[string]string{
			"RABBITMQ_DEFAULT_USER": "e2e",
			"RABBITMQ_DEFAULT_PASS": "e2e-password",
		},
		WaitingFor: wait.ForLog("Server startup complete").WithStartupTimeout(time.Minute),
	})
	return err
}

func (e *environment) startService(ctx context.Context, name string) error {
	prefix := strings.ToUpper(name) + "_"
	env := map[string]string{
		prefix + "MYSQL_ADDRESS":     name + "-mysql:3306",
		prefix + "MYSQL_DATABASE":    name + "_test",
		prefix + "MYSQL_USER":        name,
		prefix + "MYSQL_PASSWORD":    "e2e-password",
		prefix + "AMQP_URL":          "amqp://e2e:e2e-password@rabbitmq:5672/",
		prefix + "APP_GRACE_TIMEOUT": "2s",
	}
	if name == "auth" {
		env["AUTH_IDENTITY_ADDRESS"] = "identity:8081"
		env["AUTH_SESSION_ADDRESS"] = "session:8081"
	}

	container, err := e.startContainer(ctx, testcontainers.ContainerRequest{
		Image:        "distributed-programming-" + name + ":e2e",
		Cmd:          []string{"service"},
		ExposedPorts: []string{"8081/tcp", "8082/tcp"},
		Networks:     []string{e.network.Name},
		NetworkAliases: map[string][]string{
			e.network.Name: {name},
		},
		Env: env,
		WaitingFor: wait.ForHTTP("/healthz").
			WithPort("8082/tcp").
			WithStatusCodeMatcher(func(code int) bool {
				return code == http.StatusNoContent
			}).
			WithStartupTimeout(time.Minute),
	})
	if err != nil {
		if container != nil {
			printContainerLogs(container)
		}
		return err
	}
	services[name] = container
	return nil
}

func (e *environment) startContainer(
	ctx context.Context,
	request testcontainers.ContainerRequest,
) (testcontainers.Container, error) {
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: request,
		Started:          true,
	})
	if container != nil {
		e.closers = append(e.closers, func() error {
			return testcontainers.TerminateContainer(container)
		})
	}
	return container, err
}

func (e *environment) close() error {
	var errs []error
	for i := len(e.closers) - 1; i >= 0; i-- {
		errs = append(errs, e.closers[i]())
	}
	return errors.Join(errs...)
}

func printContainerLogs(container testcontainers.Container) {
	logs, err := container.Logs(context.Background())
	if err != nil {
		return
	}
	defer logs.Close()
	fmt.Fprintln(os.Stderr, "service logs:")
	_, _ = io.Copy(os.Stderr, logs)
}

func address(ctx context.Context, container testcontainers.Container, port string) (string, error) {
	host, err := container.Host(ctx)
	if err != nil {
		return "", err
	}
	mapped, err := container.MappedPort(ctx, port)
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(host, mapped.Port()), nil
}

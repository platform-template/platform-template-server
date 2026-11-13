package testenv

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/testcontainers/testcontainers-go/modules/compose"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	identityapi "identity/api/server/internalapi"
	sessionapi "session/api/server/internalapi"
)

type Environment struct {
	HTTP     *http.Client
	HTTPURLs map[string]string
	DB       map[string]*sql.DB
	Identity identityapi.IdentityServiceClient
	Session  sessionapi.SessionServiceClient

	stack       *compose.DockerCompose
	connections []*grpc.ClientConn
}

func Start(ctx context.Context, composeFile string) (_ *Environment, err error) {
	stack, err := compose.NewDockerComposeWith(compose.WithStackFiles(composeFile))
	if err != nil {
		return nil, err
	}
	env := &Environment{
		HTTP:     &http.Client{Timeout: 10 * time.Second},
		HTTPURLs: make(map[string]string),
		DB:       make(map[string]*sql.DB),
		stack:    stack,
	}
	defer func() {
		if err != nil {
			err = errors.Join(err, env.Close())
		}
	}()

	for _, name := range []string{"auth", "identity", "session"} {
		stack.WaitForService(
			name,
			wait.ForHTTP("/healthz").
				WithPort("8082/tcp").
				WithStatusCodeMatcher(func(code int) bool {
					return code == http.StatusNoContent
				}).
				WithStartupTimeout(2*time.Minute),
		)
	}
	if err = stack.Up(ctx, compose.Wait(true)); err != nil {
		return nil, fmt.Errorf("start compose: %w", err)
	}

	for _, name := range []string{"auth", "identity", "session"} {
		var addr string
		addr, err = env.address(ctx, name, "8082/tcp")
		if err != nil {
			return nil, err
		}
		env.HTTPURLs[name] = "http://" + addr

		addr, err = env.address(ctx, name+"-mysql", "3306/tcp")
		if err != nil {
			return nil, err
		}
		cfg := mysqldriver.NewConfig()
		cfg.Net, cfg.Addr = "tcp", addr
		cfg.DBName, cfg.User, cfg.Passwd = name+"_test", name, "e2e-password"
		cfg.ParseTime = true
		env.DB[name], err = sql.Open("mysql", cfg.FormatDSN())
		if err != nil {
			return nil, fmt.Errorf("open %s database: %w", name, err)
		}
	}

	identityConn, err := env.grpcConn(ctx, "identity")
	if err != nil {
		return nil, err
	}
	env.Identity = identityapi.NewIdentityServiceClient(identityConn)

	sessionConn, err := env.grpcConn(ctx, "session")
	if err != nil {
		return nil, err
	}
	env.Session = sessionapi.NewSessionServiceClient(sessionConn)
	return env, nil
}

func (e *Environment) address(ctx context.Context, name, port string) (string, error) {
	container, err := e.stack.ServiceContainer(ctx, name)
	if err != nil {
		return "", fmt.Errorf("%s container: %w", name, err)
	}
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

func (e *Environment) grpcConn(ctx context.Context, name string) (*grpc.ClientConn, error) {
	addr, err := e.address(ctx, name, "8081/tcp")
	if err != nil {
		return nil, err
	}
	conn, err := grpc.NewClient(
		addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, err
	}
	e.connections = append(e.connections, conn)
	return conn, nil
}

func (e *Environment) Close() error {
	var errs []error
	for _, conn := range e.connections {
		errs = append(errs, conn.Close())
	}
	for _, db := range e.DB {
		errs = append(errs, db.Close())
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	errs = append(errs, e.stack.Down(
		ctx,
		compose.RemoveOrphans(true),
		compose.RemoveVolumes(true),
	))
	errs = append(errs, e.stack.Close())
	return errors.Join(errs...)
}

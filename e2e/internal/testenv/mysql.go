package testenv

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	mysqldriver "github.com/go-sql-driver/mysql"
)

func (e *Environment) MySQL(ctx context.Context, service string) (*sql.DB, error) {
	return e.mysql(ctx, service, false)
}

func (e *Environment) MySQLRoot(ctx context.Context, service string) (*sql.DB, error) {
	return e.mysql(ctx, service, true)
}

func (e *Environment) mysql(ctx context.Context, service string, root bool) (*sql.DB, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil, fmt.Errorf("connect %s MySQL: environment is closed", service)
	}
	cacheKey := service
	if root {
		cacheKey += ":root"
	}
	if db := e.databases[cacheKey]; db != nil {
		return db, nil
	}
	container, err := e.stack.ServiceContainer(ctx, service)
	if err != nil {
		return nil, fmt.Errorf("find MySQL service %q: %w", service, err)
	}
	info, err := container.Inspect(ctx)
	if err != nil {
		return nil, fmt.Errorf("inspect MySQL service %q: %w", service, err)
	}
	if info.Config == nil {
		return nil, fmt.Errorf("MySQL service %q has no container configuration", service)
	}
	values := make(map[string]string)
	for _, item := range info.Config.Env {
		key, value, _ := strings.Cut(item, "=")
		if key == "MYSQL_DATABASE" || key == "MYSQL_USER" || key == "MYSQL_PASSWORD" || key == "MYSQL_ROOT_PASSWORD" {
			values[key] = value
		}
	}
	required := []string{"MYSQL_DATABASE", "MYSQL_USER", "MYSQL_PASSWORD"}
	if root {
		required = []string{"MYSQL_DATABASE", "MYSQL_ROOT_PASSWORD"}
	}
	for _, key := range required {
		if values[key] == "" {
			return nil, fmt.Errorf("MySQL service %q is missing %s", service, key)
		}
	}
	addr, err := e.address(ctx, service, "3306/tcp")
	if err != nil {
		return nil, err
	}
	cfg := mysqldriver.NewConfig()
	cfg.Net, cfg.Addr = "tcp", addr
	cfg.DBName = values["MYSQL_DATABASE"]
	cfg.User = values["MYSQL_USER"]
	cfg.Passwd = values["MYSQL_PASSWORD"]
	if root {
		cfg.User, cfg.Passwd = "root", values["MYSQL_ROOT_PASSWORD"]
	}
	cfg.ParseTime = true
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, fmt.Errorf("open MySQL pool for %q: %w", service, err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connect to MySQL service %q: %w", service, err)
	}
	e.databases[cacheKey] = db
	return db, nil
}

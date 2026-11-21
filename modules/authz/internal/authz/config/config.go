package config

import (
	"errors"
	"time"
)

type Env struct {
	App     App           `envconfig:"app"`
	GRPC    GRPC          `envconfig:"grpc"`
	HTTP    HTTP          `envconfig:"http"`
	MySQL   MySQL         `envconfig:"mysql"`
	Session RemoteService `envconfig:"session"`
}

type App struct {
	GraceTimeout time.Duration `envconfig:"grace_timeout" default:"30s"`
}
type GRPC struct {
	Address string `envconfig:"address" default:":8081"`
}
type HTTP struct {
	Address string `envconfig:"address" default:":8082"`
}
type RemoteService struct {
	Address string `envconfig:"address"`
}

type MySQL struct {
	Address               string        `envconfig:"address"                  default:"localhost:3306"`
	Database              string        `envconfig:"database"                 default:"authz"`
	User                  string        `envconfig:"user"                     default:"authz"`
	Password              string        `envconfig:"password"                 default:"authz"`
	MaxConnections        int           `envconfig:"max_connections"          default:"10"`
	ConnectionMaxLifeTime time.Duration `envconfig:"connection_max_lifetime"  default:"30m"`
	ConnectionMaxIdleTime time.Duration `envconfig:"connection_max_idle_time" default:"5m"`
	ConnectTimeout        time.Duration `envconfig:"connect_timeout"          default:"5s"`
	ReadTimeout           time.Duration `envconfig:"read_timeout"             default:"10s"`
	WriteTimeout          time.Duration `envconfig:"write_timeout"            default:"10s"`
}

func (c Env) Validate() error {
	if c.App.GraceTimeout <= 0 || c.GRPC.Address == "" ||
		c.HTTP.Address == "" ||
		c.Session.Address == "" {
		return errors.New(
			"app timeout and service addresses must not be empty",
		)
	}
	if c.MySQL.Address == "" || c.MySQL.Database == "" ||
		c.MySQL.User == "" {
		return errors.New(
			"MySQL address, database, and user must not be empty",
		)
	}
	if c.MySQL.MaxConnections <= 0 {
		return errors.New("MySQL max connections must be positive")
	}
	if c.MySQL.ConnectTimeout <= 0 || c.MySQL.ReadTimeout <= 0 ||
		c.MySQL.WriteTimeout <= 0 {
		return errors.New("MySQL timeouts must be positive")
	}
	if c.MySQL.ConnectionMaxLifeTime < 0 ||
		c.MySQL.ConnectionMaxIdleTime < 0 {
		return errors.New(
			"MySQL connection lifetimes must not be negative",
		)
	}
	return nil
}

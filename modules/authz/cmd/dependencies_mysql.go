package main

import (
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	mysqldriver "github.com/go-sql-driver/mysql"

	"authz/internal/authz/config"
)

func openDatabase(conf config.Env) (mysql.Connector, error) {
	dsn := mysqldriver.NewConfig()
	dsn.Net = "tcp"
	dsn.Addr = conf.MySQL.Address
	dsn.DBName = conf.MySQL.Database
	dsn.User = conf.MySQL.User
	dsn.Passwd = conf.MySQL.Password
	dsn.ParseTime = true
	dsn.Timeout = conf.MySQL.ConnectTimeout
	dsn.ReadTimeout = conf.MySQL.ReadTimeout
	dsn.WriteTimeout = conf.MySQL.WriteTimeout
	dsn.Params = map[string]string{"charset": "utf8mb4"}

	connector := mysql.NewConnector()
	if err := connector.Open(dsn.FormatDSN(), mysql.Config{
		MaxConnections:        conf.MySQL.MaxConnections,
		ConnectionMaxLifeTime: conf.MySQL.ConnectionMaxLifeTime,
		ConnectionMaxIdleTime: conf.MySQL.ConnectionMaxIdleTime,
	}); err != nil {
		return nil, err
	}
	return connector, nil
}

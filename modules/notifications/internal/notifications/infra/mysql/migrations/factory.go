package migrations

import (
	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

func Database(client mysql.ClientContext) []migrator.Migration {
	result := make([]migrator.Migration, 0, len(migrations))
	for _, build := range migrations {
		result = append(result, build(client))
	}
	return result
}

type mysqlMigration func(mysql.ClientContext) migrator.Migration

var migrations = []mysqlMigration{
	Version1791225168,
	Version1800000000,
}

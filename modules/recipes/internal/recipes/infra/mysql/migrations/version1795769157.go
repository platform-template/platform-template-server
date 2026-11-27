package migrations

import (
	"context"
	"fmt"

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

func Version1795769157(client mysql.ClientContext) migrator.Migration {
	return &version1795769157{
		client: client,
	}
}

type version1795769157 struct {
	client mysql.ClientContext
}

func (v *version1795769157) Version() int64 {
	return 1795769157
}

func (v *version1795769157) Description() string {
	return "Create recipes and ingredients"
}

func (v *version1795769157) Up(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS recipes (
			id BINARY(16) NOT NULL PRIMARY KEY,
			name VARCHAR(255) NOT NULL,
			description TEXT NOT NULL,
			instructions MEDIUMTEXT NOT NULL,
			servings BIGINT NOT NULL,
			archived BOOLEAN NOT NULL DEFAULT FALSE,
			CHECK (servings > 0),
			KEY recipes_name (name, id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS recipe_ingredients (
			recipe_id BINARY(16) NOT NULL,
			product_id BINARY(16) NOT NULL,
			quantity BIGINT NOT NULL,
			PRIMARY KEY (recipe_id, product_id),
			KEY recipe_ingredients_product (product_id),
			CHECK (quantity > 0),
			CONSTRAINT recipe_ingredients_recipe_fk FOREIGN KEY (recipe_id) REFERENCES recipes(id)
		) ENGINE=InnoDB`,
		`CREATE TABLE IF NOT EXISTS recipe_owners (
			recipe_id BINARY(16) NOT NULL PRIMARY KEY,
			user_id BINARY(16) NOT NULL,
			KEY recipe_owners_user (user_id, recipe_id),
			CONSTRAINT recipe_owners_recipe_fk FOREIGN KEY (recipe_id) REFERENCES recipes(id)
		) ENGINE=InnoDB`,
	}
	for _, statement := range statements {
		if _, err := v.client.ExecContext(ctx, "recipes.create_schema", statement); err != nil {
			return fmt.Errorf("create recipe schema: %w", err)
		}
	}
	return nil
}

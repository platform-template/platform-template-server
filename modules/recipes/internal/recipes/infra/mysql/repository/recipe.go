package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"

	"recipes/internal/recipes/domain"
)

type recipeRepository struct {
	ctx    context.Context
	client mysql.ClientContext
}

func NewRecipeRepository(ctx context.Context, client mysql.ClientContext) domain.RecipeRepository {
	return &recipeRepository{ctx: ctx, client: client}
}

func (r *recipeRepository) Create(recipe *domain.Recipe) error {
	if recipe == nil {
		return domain.ErrInvalidRecipe
	}
	data := recipe.Data()
	if err := validateStorage(data); err != nil {
		return err
	}
	_, err := r.client.ExecContext(r.ctx, "recipe.create", `INSERT INTO recipes
		(id, name, description, instructions, servings, archived) VALUES (?, ?, ?, ?, ?, ?)`,
		data.ID[:], data.Name, data.Description, data.Instructions, data.Servings, data.Archived)
	if err != nil {
		return fmt.Errorf("create recipe: %w", err)
	}
	return r.insertIngredients(data)
}

func (r *recipeRepository) Update(recipe *domain.Recipe) error {
	if recipe == nil {
		return domain.ErrInvalidRecipe
	}
	data := recipe.Data()
	if err := validateStorage(data); err != nil {
		return err
	}
	var archived bool
	err := r.client.QueryRowContext(r.ctx, "recipe.lock", `SELECT archived FROM recipes WHERE id = ? FOR UPDATE`, data.ID[:]).
		Scan(&archived)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrRecipeNotFound
	}
	if err != nil {
		return fmt.Errorf("lock recipe: %w", err)
	}
	if archived {
		return domain.ErrRecipeArchived
	}
	_, err = r.client.ExecContext(r.ctx, "recipe.update", `UPDATE recipes SET name = ?, description = ?,
		instructions = ?, servings = ?, archived = ? WHERE id = ?`, data.Name, data.Description,
		data.Instructions, data.Servings, data.Archived, data.ID[:])
	if err != nil {
		return fmt.Errorf("update recipe: %w", err)
	}
	_, err = r.client.ExecContext(
		r.ctx,
		"recipe.ingredients.delete",
		`DELETE FROM recipe_ingredients WHERE recipe_id = ?`,
		data.ID[:],
	)
	if err != nil {
		return fmt.Errorf("replace recipe ingredients: %w", err)
	}
	return r.insertIngredients(data)
}

func (r *recipeRepository) insertIngredients(data domain.RecipeData) error {
	for _, ingredient := range data.Ingredients {
		_, err := r.client.ExecContext(r.ctx, "recipe.ingredient.insert", `INSERT INTO recipe_ingredients
			(recipe_id, product_id, quantity) VALUES (?, ?, ?)`, data.ID[:], ingredient.ProductID[:], ingredient.Quantity)
		if err != nil {
			return fmt.Errorf("insert recipe ingredient: %w", err)
		}
	}
	return nil
}

func (r *recipeRepository) GetByID(id uuid.UUID) (*domain.Recipe, error) {
	var data domain.RecipeData
	data.ID = id
	err := r.client.QueryRowContext(r.ctx, "recipe.get", `SELECT name, description, instructions, servings, archived
		FROM recipes WHERE id = ?`, id[:]).Scan(&data.Name, &data.Description, &data.Instructions, &data.Servings, &data.Archived)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get recipe: %w", err)
	}
	return r.restore(data)
}

func (r *recipeRepository) List(filter domain.RecipeFilter) ([]*domain.Recipe, error) {
	return r.list(filter, nil)
}

func (r *recipeRepository) ListByOwner(ownerID uuid.UUID, filter domain.RecipeFilter) ([]*domain.Recipe, error) {
	if ownerID == uuid.Nil() {
		return nil, domain.ErrInvalidRecipe
	}
	return r.list(filter, &ownerID)
}

func (r *recipeRepository) list(filter domain.RecipeFilter, ownerID *uuid.UUID) ([]*domain.Recipe, error) {
	if filter.Limit <= 0 || filter.Limit > 100 || filter.Offset < 0 {
		return nil, domain.ErrInvalidRecipe
	}
	escaped := strings.NewReplacer("!", "!!", "%", "!%", "_", "!_").Replace(filter.Search)
	query := `SELECT r.id, r.name, r.description, r.instructions, r.servings, r.archived FROM recipes r`
	args := make([]any, 0, 4)
	if ownerID != nil {
		query += ` JOIN recipe_owners o ON o.recipe_id = r.id AND o.user_id = ?`
		args = append(args, ownerID[:])
	}
	query += ` WHERE r.archived = FALSE AND r.name COLLATE utf8mb4_0900_ai_ci LIKE ? ESCAPE '!'
		ORDER BY r.name COLLATE utf8mb4_0900_ai_ci, r.id LIMIT ? OFFSET ?`
	args = append(args, "%"+escaped+"%", filter.Limit, filter.Offset)
	rows, err := r.client.QueryContext(r.ctx, "recipe.list", query, args...)
	if err != nil {
		return nil, fmt.Errorf("list recipes: %w", err)
	}
	defer rows.Close()
	var dataList []domain.RecipeData
	for rows.Next() {
		var id []byte
		var data domain.RecipeData
		if err := rows.Scan(
			&id,
			&data.Name,
			&data.Description,
			&data.Instructions,
			&data.Servings,
			&data.Archived,
		); err != nil {
			return nil, fmt.Errorf("scan recipe: %w", err)
		}
		if len(id) != 16 {
			return nil, errors.New("invalid recipe ID")
		}
		copy(data.ID[:], id)
		dataList = append(dataList, data)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recipes: %w", err)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close recipes: %w", err)
	}
	recipes := make([]*domain.Recipe, 0, len(dataList))
	for _, data := range dataList {
		recipe, err := r.restore(data)
		if err != nil {
			return nil, err
		}
		recipes = append(recipes, recipe)
	}
	return recipes, nil
}

func (r *recipeRepository) restore(data domain.RecipeData) (*domain.Recipe, error) {
	rows, err := r.client.QueryContext(r.ctx, "recipe.ingredients", `SELECT product_id, quantity
		FROM recipe_ingredients WHERE recipe_id = ? ORDER BY product_id`, data.ID[:])
	if err != nil {
		return nil, fmt.Errorf("get recipe ingredients: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id []byte
		var item domain.Ingredient
		if err := rows.Scan(&id, &item.Quantity); err != nil {
			return nil, fmt.Errorf("scan ingredient: %w", err)
		}
		if len(id) != 16 {
			return nil, errors.New("invalid ingredient product ID")
		}
		copy(item.ProductID[:], id)
		data.Ingredients = append(data.Ingredients, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ingredients: %w", err)
	}
	recipe, err := domain.RestoreRecipe(data)
	if err != nil {
		return nil, fmt.Errorf("restore recipe: %w", err)
	}
	return recipe, nil
}

func validateStorage(data domain.RecipeData) error {
	if !utf8.ValidString(data.Name) || utf8.RuneCountInString(data.Name) > 255 ||
		!utf8.ValidString(data.Description) || len(data.Description) > 65535 ||
		!utf8.ValidString(data.Instructions) || len(data.Instructions) > 16777215 {
		return domain.ErrInvalidRecipe
	}
	return nil
}

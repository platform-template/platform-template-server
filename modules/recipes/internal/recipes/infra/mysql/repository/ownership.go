package repository

import (
	"context"
	"fmt"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"

	"recipes/internal/recipes/domain"
)

type ownershipRepository struct {
	ctx    context.Context
	client mysql.ClientContext
}

func NewOwnershipRepository(ctx context.Context, client mysql.ClientContext) domain.RecipeOwnership {
	return &ownershipRepository{ctx: ctx, client: client}
}

func (r *ownershipRepository) Create(recipeID, userID uuid.UUID) error {
	if recipeID == uuid.Nil() || userID == uuid.Nil() {
		return domain.ErrInvalidRecipe
	}
	_, err := r.client.ExecContext(
		r.ctx,
		"recipe.owner.create",
		`INSERT INTO recipe_owners (recipe_id, user_id) VALUES (?, ?)`,
		recipeID[:],
		userID[:],
	)
	if err != nil {
		return fmt.Errorf("create recipe owner: %w", err)
	}
	return nil
}

func (r *ownershipRepository) IsOwner(recipeID, userID uuid.UUID) (bool, error) {
	if recipeID == uuid.Nil() || userID == uuid.Nil() {
		return false, nil
	}
	var count int
	err := r.client.QueryRowContext(r.ctx, "recipe.owner.check", `SELECT COUNT(*) FROM recipe_owners WHERE recipe_id = ? AND user_id = ?`, recipeID[:], userID[:]).
		Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check recipe owner: %w", err)
	}
	return count != 0, nil
}

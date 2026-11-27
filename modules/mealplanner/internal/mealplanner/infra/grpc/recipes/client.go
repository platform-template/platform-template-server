package recipes

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"uuid"

	"mealplanner/internal/mealplanner/domain"
	recipesapi "recipes/api/server/internalapi"
)

var ErrInvalidSnapshot = errors.New("invalid recipe snapshot")

type Snapshot struct {
	RecipeID     uuid.UUID
	Name         string
	Description  string
	Instructions string
	Servings     int64
	Ingredients  []domain.Ingredient
}
type Client struct {
	api recipesapi.RecipesServiceClient
}

func NewClient(api recipesapi.RecipesServiceClient) *Client { return &Client{api: api} }
func (c *Client) Snapshot(ctx context.Context, userID, recipeID uuid.UUID, servings int64) (Snapshot, error) {
	if userID == uuid.Nil() || recipeID == uuid.Nil() || servings <= 0 {
		return Snapshot{}, ErrInvalidSnapshot
	}
	response, err := c.api.GetRecipeSnapshot(
		ctx,
		&recipesapi.GetRecipeSnapshotRequest{UserId: userID.String(), RecipeId: recipeID.String(), Servings: servings},
	)
	if err != nil {
		return Snapshot{}, fmt.Errorf("get recipe snapshot: %w", err)
	}
	if response == nil || response.GetRecipeId() != recipeID.String() || response.GetServings() != servings ||
		strings.TrimSpace(response.GetName()) == "" ||
		len(response.GetIngredients()) == 0 {
		return Snapshot{}, ErrInvalidSnapshot
	}
	snapshot := Snapshot{
		RecipeID:     recipeID,
		Name:         response.GetName(),
		Description:  response.GetDescription(),
		Instructions: response.GetInstructions(),
		Servings:     servings,
		Ingredients:  make([]domain.Ingredient, 0, len(response.GetIngredients())),
	}
	seen := make(map[uuid.UUID]bool, len(response.GetIngredients()))
	for _, ingredient := range response.GetIngredients() {
		id, err := uuid.Parse(ingredient.GetProductId())
		if err != nil || id == uuid.Nil() || ingredient.GetQuantity() <= 0 || seen[id] {
			return Snapshot{}, ErrInvalidSnapshot
		}
		seen[id] = true
		snapshot.Ingredients = append(
			snapshot.Ingredients,
			domain.Ingredient{ProductID: id, Quantity: ingredient.GetQuantity()},
		)
	}
	return snapshot, nil
}

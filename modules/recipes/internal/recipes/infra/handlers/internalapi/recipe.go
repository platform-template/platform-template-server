package internalapi

import (
	"context"
	"errors"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/logging"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	recipesinternalapi "recipes/api/server/internalapi"
	"recipes/internal/recipes/domain"
)

func (h handler) GetRecipeSnapshot(
	ctx context.Context,
	req *recipesinternalapi.GetRecipeSnapshotRequest,
) (*recipesinternalapi.RecipeSnapshot, error) {
	userID, err := uuid.Parse(req.GetUserId())
	if err != nil || userID == uuid.Nil() {
		return nil, status.Error(codes.InvalidArgument, "invalid user ID")
	}
	recipeID, err := uuid.Parse(req.GetRecipeId())
	if err != nil || recipeID == uuid.Nil() {
		return nil, status.Error(codes.InvalidArgument, "invalid recipe ID")
	}
	if req.GetServings() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "invalid servings")
	}
	recipe, err := h.svc.GetRecipe(ctx, userID, recipeID)
	if errors.Is(err, domain.ErrRecipeNotFound) {
		return nil, status.Error(codes.NotFound, "recipe not found")
	}
	if err != nil {
		h.logger.ErrorContext(ctx, "get recipe snapshot failed", logging.Error(err))
		return nil, status.Error(codes.Internal, "get recipe snapshot failed")
	}
	ingredients, err := recipe.ScaledIngredients(req.GetServings())
	if errors.Is(err, domain.ErrQuantityOverflow) || errors.Is(err, domain.ErrInvalidServings) {
		return nil, status.Error(codes.InvalidArgument, "invalid scaled quantities")
	}
	if err != nil {
		h.logger.ErrorContext(ctx, "scale recipe failed", logging.Error(err))
		return nil, status.Error(codes.Internal, "scale recipe failed")
	}
	data := recipe.Data()
	response := &recipesinternalapi.RecipeSnapshot{
		RecipeId:     data.ID.String(),
		Name:         data.Name,
		Description:  data.Description,
		Instructions: data.Instructions,
		Servings:     req.GetServings(),
		Ingredients:  make([]*recipesinternalapi.RecipeIngredient, 0, len(ingredients)),
	}
	for _, item := range ingredients {
		response.Ingredients = append(
			response.Ingredients,
			&recipesinternalapi.RecipeIngredient{ProductId: item.ProductID.String(), Quantity: item.Quantity},
		)
	}
	return response, nil
}

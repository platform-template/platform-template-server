package public

import (
	"context"
	"errors"
	"uuid"

	publicapi "recipes/api/server/publicapi"
	"recipes/internal/recipes/domain"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func recipeInput(input *publicapi.RecipeInput) (domain.RecipeData, error) {
	if input == nil {
		return domain.RecipeData{}, domain.ErrInvalidRecipe
	}
	ingredients := make([]domain.Ingredient, 0, len(input.Ingredients))
	for _, item := range input.Ingredients {
		id, err := uuid.Parse(item.ProductId)
		if err != nil {
			return domain.RecipeData{}, domain.ErrInvalidRecipe
		}
		ingredients = append(ingredients, domain.Ingredient{ProductID: id, Quantity: item.Quantity})
	}
	return domain.RecipeData{
		Name:         input.Name,
		Description:  input.Description,
		Instructions: input.Instructions,
		Servings:     input.Servings,
		Ingredients:  ingredients,
	}, nil
}
func recipeOutput(recipe *domain.Recipe) publicapi.Recipe {
	data := recipe.Data()
	return publicapi.Recipe{
		Id:           data.ID.String(),
		Name:         data.Name,
		Description:  data.Description,
		Instructions: data.Instructions,
		Servings:     data.Servings,
		Ingredients:  ingredientsOutput(data.Ingredients),
	}
}
func ingredientsOutput(items []domain.Ingredient) []publicapi.Ingredient {
	result := make([]publicapi.Ingredient, 0, len(items))
	for _, item := range items {
		result = append(result, publicapi.Ingredient{ProductId: item.ProductID.String(), Quantity: item.Quantity})
	}
	return result
}
func userID(ctx context.Context) (uuid.UUID, error) {
	id, ok := ctx.Value(userIDKey{}).(uuid.UUID)
	if !ok || id == uuid.Nil() {
		return uuid.Nil(), errors.New("missing authenticated user")
	}
	return id, nil
}
func invalidRecipe(err error) bool {
	return errors.Is(err, domain.ErrInvalidRecipe) || errors.Is(err, domain.ErrProductNotFound) ||
		errors.Is(err, domain.ErrInvalidServings) ||
		errors.Is(err, domain.ErrQuantityOverflow)
}
func unavailable(err error) bool {
	return status.Code(err) == codes.Unavailable || status.Code(err) == codes.DeadlineExceeded ||
		errors.Is(err, context.DeadlineExceeded)
}

func (h handler) ListRecipes(
	ctx context.Context,
	req publicapi.ListRecipesRequestObject,
) (publicapi.ListRecipesResponseObject, error) {
	owner, err := userID(ctx)
	if err != nil {
		return nil, err
	}
	filter := domain.RecipeFilter{}
	if req.Params.Search != nil {
		filter.Search = *req.Params.Search
	}
	if req.Params.Limit != nil {
		filter.Limit = *req.Params.Limit
	}
	if req.Params.Offset != nil {
		filter.Offset = *req.Params.Offset
	}
	items, err := h.svc.ListRecipes(ctx, owner, filter)
	if err != nil {
		return nil, err
	}
	result := make([]publicapi.Recipe, 0, len(items))
	for _, item := range items {
		result = append(result, recipeOutput(item))
	}
	return publicapi.ListRecipes200JSONResponse{Recipes: result}, nil
}

func (h handler) CreateRecipe(
	ctx context.Context,
	req publicapi.CreateRecipeRequestObject,
) (publicapi.CreateRecipeResponseObject, error) {
	owner, err := userID(ctx)
	if err != nil {
		return nil, err
	}
	data, err := recipeInput(req.Body)
	if err != nil {
		return publicapi.CreateRecipe400Response{}, nil
	}
	recipe, err := h.svc.CreateRecipe(ctx, owner, data)
	if invalidRecipe(err) {
		return publicapi.CreateRecipe400Response{}, nil
	}
	if unavailable(err) {
		return publicapi.CreateRecipe503Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.CreateRecipe201JSONResponse(recipeOutput(recipe)), nil
}

func (h handler) GetRecipe(
	ctx context.Context,
	req publicapi.GetRecipeRequestObject,
) (publicapi.GetRecipeResponseObject, error) {
	owner, err := userID(ctx)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.Id)
	if err != nil || id == uuid.Nil() {
		return publicapi.GetRecipe400Response{}, nil
	}
	recipe, err := h.svc.GetRecipe(ctx, owner, id)
	if errors.Is(err, domain.ErrRecipeNotFound) {
		return publicapi.GetRecipe404Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.GetRecipe200JSONResponse(recipeOutput(recipe)), nil
}

func (h handler) ReviseRecipe(
	ctx context.Context,
	req publicapi.ReviseRecipeRequestObject,
) (publicapi.ReviseRecipeResponseObject, error) {
	owner, err := userID(ctx)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.Id)
	if err != nil || id == uuid.Nil() {
		return publicapi.ReviseRecipe400Response{}, nil
	}
	data, err := recipeInput(req.Body)
	if err != nil {
		return publicapi.ReviseRecipe400Response{}, nil
	}
	err = h.svc.ReviseRecipe(ctx, owner, id, data)
	if errors.Is(err, domain.ErrRecipeNotFound) {
		return publicapi.ReviseRecipe404Response{}, nil
	}
	if invalidRecipe(err) {
		return publicapi.ReviseRecipe400Response{}, nil
	}
	if unavailable(err) {
		return publicapi.ReviseRecipe503Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.ReviseRecipe204Response{}, nil
}

func (h handler) ArchiveRecipe(
	ctx context.Context,
	req publicapi.ArchiveRecipeRequestObject,
) (publicapi.ArchiveRecipeResponseObject, error) {
	owner, err := userID(ctx)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.Id)
	if err != nil || id == uuid.Nil() {
		return publicapi.ArchiveRecipe400Response{}, nil
	}
	err = h.svc.ArchiveRecipe(ctx, owner, id)
	if errors.Is(err, domain.ErrRecipeNotFound) {
		return publicapi.ArchiveRecipe404Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.ArchiveRecipe204Response{}, nil
}

func (h handler) ScaleIngredients(
	ctx context.Context,
	req publicapi.ScaleIngredientsRequestObject,
) (publicapi.ScaleIngredientsResponseObject, error) {
	owner, err := userID(ctx)
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(req.Id)
	if err != nil || id == uuid.Nil() {
		return publicapi.ScaleIngredients400Response{}, nil
	}
	recipe, err := h.svc.GetRecipe(ctx, owner, id)
	if errors.Is(err, domain.ErrRecipeNotFound) {
		return publicapi.ScaleIngredients404Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	ingredients, err := recipe.ScaledIngredients(req.Params.Servings)
	if invalidRecipe(err) {
		return publicapi.ScaleIngredients400Response{}, nil
	}
	if err != nil {
		return nil, err
	}
	return publicapi.ScaleIngredients200JSONResponse{Ingredients: ingredientsOutput(ingredients)}, nil
}

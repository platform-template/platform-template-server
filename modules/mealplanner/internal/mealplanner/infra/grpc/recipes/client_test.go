package recipes

import (
	"context"
	"errors"
	"testing"
	"uuid"

	recipesapi "recipes/api/server/internalapi"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

type fakeRecipes struct {
	recipesapi.RecipesServiceClient
	response *recipesapi.RecipeSnapshot
	err      error
	request  *recipesapi.GetRecipeSnapshotRequest
}

func (f *fakeRecipes) GetRecipeSnapshot(
	_ context.Context,
	req *recipesapi.GetRecipeSnapshotRequest,
	_ ...grpc.CallOption,
) (*recipesapi.RecipeSnapshot, error) {
	f.request = req
	return f.response, f.err
}
func TestSnapshot(t *testing.T) {
	user, recipe, product := uuid.New(), uuid.New(), uuid.New()
	api := &fakeRecipes{
		response: &recipesapi.RecipeSnapshot{
			RecipeId:    recipe.String(),
			Name:        "Soup",
			Servings:    2,
			Ingredients: []*recipesapi.RecipeIngredient{{ProductId: product.String(), Quantity: 5}},
		},
	}
	client := NewClient(api)
	snapshot, err := client.Snapshot(t.Context(), user, recipe, 2)
	require.NoError(t, err)
	require.Equal(t, user.String(), api.request.GetUserId())
	require.Equal(t, recipe.String(), api.request.GetRecipeId())
	require.Equal(t, product, snapshot.Ingredients[0].ProductID)
	require.EqualValues(t, 5, snapshot.Ingredients[0].Quantity)
	api.response.Ingredients[0].Quantity = 20
	require.EqualValues(t, 5, snapshot.Ingredients[0].Quantity)
	api.response.Ingredients = append(
		api.response.Ingredients,
		&recipesapi.RecipeIngredient{ProductId: product.String(), Quantity: 1},
	)
	_, err = client.Snapshot(t.Context(), user, recipe, 2)
	require.ErrorIs(t, err, ErrInvalidSnapshot)
	api.response = nil
	_, err = client.Snapshot(t.Context(), user, recipe, 2)
	require.ErrorIs(t, err, ErrInvalidSnapshot)
	failure := errors.New("unavailable")
	api.err = failure
	_, err = client.Snapshot(t.Context(), user, recipe, 2)
	require.ErrorIs(t, err, failure)
}

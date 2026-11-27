package domain

import (
	"math"
	"testing"
	"uuid"

	"github.com/stretchr/testify/require"
)

func TestRecipeScaling(t *testing.T) {
	product := uuid.New()
	recipe, err := NewRecipe(RecipeData{ID: uuid.New(), Name: "Греча", Servings: 3,
		Ingredients: []Ingredient{{ProductID: product, Quantity: 5}}})
	require.NoError(t, err)

	for _, test := range []struct{ servings, quantity int64 }{{1, 2}, {3, 5}, {4, 7}, {6, 10}} {
		items, err := recipe.ScaledIngredients(test.servings)
		require.NoError(t, err)
		require.Equal(t, []Ingredient{{ProductID: product, Quantity: test.quantity}}, items)
	}
	_, err = recipe.ScaledIngredients(0)
	require.ErrorIs(t, err, ErrInvalidServings)
}

func TestScalingOverflow(t *testing.T) {
	product := uuid.New()
	recipe, err := NewRecipe(RecipeData{ID: uuid.New(), Name: "Каша", Servings: math.MaxInt64,
		Ingredients: []Ingredient{{ProductID: product, Quantity: math.MaxInt64}}})
	require.NoError(t, err)
	items, err := recipe.ScaledIngredients(math.MaxInt64)
	require.NoError(t, err)
	require.EqualValues(t, math.MaxInt64, items[0].Quantity)

	recipe, err = NewRecipe(RecipeData{ID: uuid.New(), Name: "Каша", Servings: 1,
		Ingredients: []Ingredient{{ProductID: product, Quantity: math.MaxInt64}}})
	require.NoError(t, err)
	_, err = recipe.ScaledIngredients(2)
	require.ErrorIs(t, err, ErrQuantityOverflow)
}

func TestRecipeValidationAndIsolation(t *testing.T) {
	id, product := uuid.New(), uuid.New()
	input := RecipeData{
		ID:          id,
		Name:        "Борщ",
		Servings:    4,
		Ingredients: []Ingredient{{ProductID: product, Quantity: 10}},
	}
	recipe, err := NewRecipe(input)
	require.NoError(t, err)
	input.Ingredients[0].Quantity = 99
	snapshot := recipe.Data()
	snapshot.Ingredients[0].Quantity = 88
	require.EqualValues(t, 10, recipe.Data().Ingredients[0].Quantity)

	err = recipe.Revise(" ", "", "", 2, []Ingredient{{ProductID: product, Quantity: 2}})
	require.ErrorIs(t, err, ErrInvalidRecipe)
	require.Equal(t, "Борщ", recipe.Data().Name)
	err = recipe.Revise("Щи", "", "Варить", 2, []Ingredient{{ProductID: product, Quantity: 2}})
	require.NoError(t, err)
	require.Equal(t, id, recipe.Data().ID)
	require.True(t, recipe.Archive())
	require.False(t, recipe.Archive())
	require.ErrorIs(
		t,
		recipe.Revise("Новый", "", "", 1, []Ingredient{{ProductID: product, Quantity: 1}}),
		ErrRecipeArchived,
	)
}

func TestRecipeRejectsInvalidIngredients(t *testing.T) {
	product := uuid.New()
	for _, items := range [][]Ingredient{
		nil,
		{{ProductID: uuid.Nil(), Quantity: 1}},
		{{ProductID: product, Quantity: 0}},
		{{ProductID: product, Quantity: 1}, {ProductID: product, Quantity: 2}},
	} {
		_, err := NewRecipe(RecipeData{ID: uuid.New(), Name: "Борщ", Servings: 1, Ingredients: items})
		require.ErrorIs(t, err, ErrInvalidRecipe)
	}
}

package domain

import (
	"errors"
	"math/big"
	"sort"
	"strings"
	"uuid"
)

var (
	ErrInvalidRecipe    = errors.New("invalid recipe")
	ErrRecipeNotFound   = errors.New("recipe not found")
	ErrRecipeArchived   = errors.New("recipe archived")
	ErrInvalidServings  = errors.New("invalid servings")
	ErrQuantityOverflow = errors.New("scaled ingredient quantity overflows")
	ErrProductNotFound  = errors.New("ingredient product not found")
)

type Ingredient struct {
	ProductID uuid.UUID
	Quantity  int64
}

type RecipeData struct {
	ID           uuid.UUID
	Name         string
	Description  string
	Servings     int64
	Ingredients  []Ingredient
	Instructions string
	Archived     bool
}

type Recipe struct{ data RecipeData }

func NewRecipe(data RecipeData) (*Recipe, error) {
	if data.Archived {
		return nil, ErrInvalidRecipe
	}
	return RestoreRecipe(data)
}

func RestoreRecipe(data RecipeData) (*Recipe, error) {
	if data.ID == uuid.Nil() || strings.TrimSpace(data.Name) == "" || data.Servings <= 0 || len(data.Ingredients) == 0 {
		return nil, ErrInvalidRecipe
	}
	data.Ingredients = append([]Ingredient(nil), data.Ingredients...)
	sort.Slice(data.Ingredients, func(i, j int) bool {
		return data.Ingredients[i].ProductID.String() < data.Ingredients[j].ProductID.String()
	})
	for i, ingredient := range data.Ingredients {
		if ingredient.ProductID == uuid.Nil() || ingredient.Quantity <= 0 ||
			(i > 0 && data.Ingredients[i-1].ProductID == ingredient.ProductID) {
			return nil, ErrInvalidRecipe
		}
	}
	return &Recipe{data: data}, nil
}

func (r *Recipe) Data() RecipeData {
	data := r.data
	data.Ingredients = append([]Ingredient(nil), data.Ingredients...)
	return data
}

func (r *Recipe) Revise(name, description, instructions string, servings int64, ingredients []Ingredient) error {
	if r.data.Archived {
		return ErrRecipeArchived
	}
	updated, err := NewRecipe(RecipeData{
		ID: r.data.ID, Name: name, Description: description, Instructions: instructions,
		Servings: servings, Ingredients: ingredients,
	})
	if err != nil {
		return err
	}
	r.data = updated.data
	return nil
}

func (r *Recipe) Archive() bool {
	if r.data.Archived {
		return false
	}
	r.data.Archived = true
	return true
}

// ScaledIngredients rounds each positive fractional quantity up to avoid a shortage.
func (r *Recipe) ScaledIngredients(servings int64) ([]Ingredient, error) {
	if servings <= 0 {
		return nil, ErrInvalidServings
	}
	scaled := make([]Ingredient, 0, len(r.data.Ingredients))
	for _, ingredient := range r.data.Ingredients {
		numerator := new(big.Int).Mul(big.NewInt(ingredient.Quantity), big.NewInt(servings))
		quantity, remainder := new(big.Int), new(big.Int)
		quantity.QuoRem(numerator, big.NewInt(r.data.Servings), remainder)
		if remainder.Sign() != 0 {
			quantity.Add(quantity, big.NewInt(1))
		}
		if !quantity.IsInt64() {
			return nil, ErrQuantityOverflow
		}
		scaled = append(scaled, Ingredient{ProductID: ingredient.ProductID, Quantity: quantity.Int64()})
	}
	return scaled, nil
}

type RecipeFilter struct {
	Search string
	Limit  int
	Offset int
}

type RecipeRepository interface {
	Create(*Recipe) error
	Update(*Recipe) error
	GetByID(uuid.UUID) (*Recipe, error)
	List(RecipeFilter) ([]*Recipe, error)
	ListByOwner(uuid.UUID, RecipeFilter) ([]*Recipe, error)
}

type RecipeOwnership interface {
	Create(recipeID, userID uuid.UUID) error
	IsOwner(recipeID, userID uuid.UUID) (bool, error)
}

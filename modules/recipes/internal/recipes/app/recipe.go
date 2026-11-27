package app

import (
	"context"
	"errors"
	"fmt"
	"time"
	"uuid"

	recipesevents "recipes/api/events"
	"recipes/internal/recipes/domain"
	"recipes/internal/recipes/infra/mysql"
)

func (s *Service) validateProducts(ctx context.Context, ingredients []domain.Ingredient) error {
	if s.catalog == nil {
		return errors.New("catalog client is not configured")
	}
	validationCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for _, ingredient := range ingredients {
		if err := s.catalog.ValidateProduct(validationCtx, ingredient.ProductID); err != nil {
			return fmt.Errorf("validate ingredient %s: %w", ingredient.ProductID, err)
		}
	}
	return nil
}

func (s *Service) CreateRecipe(ctx context.Context, ownerID uuid.UUID, data domain.RecipeData) (*domain.Recipe, error) {
	if ownerID == uuid.Nil() {
		return nil, domain.ErrInvalidRecipe
	}
	data.ID = uuid.New()
	data.Archived = false
	recipe, err := domain.NewRecipe(data)
	if err != nil {
		return nil, err
	}
	if err := s.validateProducts(ctx, recipe.Data().Ingredients); err != nil {
		return nil, err
	}
	err = s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		if err := p.Recipe(ctx).Create(recipe); err != nil {
			return err
		}
		if err := p.RecipeOwnership(ctx).Create(data.ID, ownerID); err != nil {
			return err
		}
		if s.dispatcherFactory == nil {
			return errors.New("recipe event dispatcher is not configured")
		}
		return s.dispatch(ctx, &recipesevents.RecipeCreated{
			RecipeId: data.ID.String(), ActorId: ownerID.String(), Name: data.Name,
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create recipe: %w", err)
	}
	return recipe, nil
}

func (s *Service) GetRecipe(ctx context.Context, ownerID, recipeID uuid.UUID) (*domain.Recipe, error) {
	var recipe *domain.Recipe
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		owned, err := p.RecipeOwnership(ctx).IsOwner(recipeID, ownerID)
		if err != nil {
			return err
		}
		if !owned {
			return domain.ErrRecipeNotFound
		}
		recipe, err = p.Recipe(ctx).GetByID(recipeID)
		if err != nil {
			return err
		}
		if recipe == nil || recipe.Data().Archived {
			return domain.ErrRecipeNotFound
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("get recipe: %w", err)
	}
	return recipe, nil
}

func (s *Service) ListRecipes(
	ctx context.Context,
	ownerID uuid.UUID,
	filter domain.RecipeFilter,
) ([]*domain.Recipe, error) {
	if ownerID == uuid.Nil() || filter.Limit < 0 || filter.Limit > 100 || filter.Offset < 0 {
		return nil, domain.ErrInvalidRecipe
	}
	if filter.Limit == 0 {
		filter.Limit = 20
	}
	var recipes []*domain.Recipe
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		var err error
		recipes, err = p.Recipe(ctx).ListByOwner(ownerID, filter)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list recipes: %w", err)
	}
	return recipes, nil
}

func (s *Service) ReviseRecipe(ctx context.Context, ownerID, recipeID uuid.UUID, data domain.RecipeData) error {
	data.ID, data.Archived = recipeID, false
	candidate, err := domain.NewRecipe(data)
	if err != nil {
		return err
	}
	if err := s.validateProducts(ctx, candidate.Data().Ingredients); err != nil {
		return err
	}
	err = s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		owned, err := p.RecipeOwnership(ctx).IsOwner(recipeID, ownerID)
		if err != nil {
			return err
		}
		if !owned {
			return domain.ErrRecipeNotFound
		}
		current, err := p.Recipe(ctx).GetByID(recipeID)
		if err != nil {
			return err
		}
		if current == nil || current.Data().Archived {
			return domain.ErrRecipeNotFound
		}
		if err := current.Revise(
			data.Name,
			data.Description,
			data.Instructions,
			data.Servings,
			data.Ingredients,
		); err != nil {
			return err
		}
		if err := p.Recipe(ctx).Update(current); err != nil {
			return err
		}
		if s.dispatcherFactory == nil {
			return errors.New("recipe event dispatcher is not configured")
		}
		return s.dispatch(ctx, &recipesevents.RecipeRevised{
			RecipeId: recipeID.String(), ActorId: ownerID.String(), Name: data.Name,
		})
	})
	if err != nil {
		return fmt.Errorf("revise recipe: %w", err)
	}
	return nil
}

func (s *Service) ArchiveRecipe(ctx context.Context, ownerID, recipeID uuid.UUID) error {
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(p *mysql.RepositoryProvider) error {
		owned, err := p.RecipeOwnership(ctx).IsOwner(recipeID, ownerID)
		if err != nil {
			return err
		}
		if !owned {
			return domain.ErrRecipeNotFound
		}
		recipe, err := p.Recipe(ctx).GetByID(recipeID)
		if err != nil {
			return err
		}
		if recipe == nil {
			return domain.ErrRecipeNotFound
		}
		if !recipe.Archive() {
			return nil
		}
		if err := p.Recipe(ctx).Update(recipe); err != nil {
			if errors.Is(err, domain.ErrRecipeArchived) {
				return nil
			}
			return err
		}
		if s.dispatcherFactory == nil {
			return errors.New("recipe event dispatcher is not configured")
		}
		return s.dispatch(ctx, &recipesevents.RecipeArchived{
			RecipeId: recipeID.String(), ActorId: ownerID.String(),
		})
	})
	if err != nil {
		return fmt.Errorf("archive recipe: %w", err)
	}
	return nil
}

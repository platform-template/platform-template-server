package persistence_tests

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/event"
	"github.com/distributed-programming-2026/go-sdk/pkg/event/outbox"
	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	"github.com/stretchr/testify/require"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"

	"recipes/internal/recipes/app"
	"recipes/internal/recipes/domain"
	eventfactory "recipes/internal/recipes/infra/event"
	recipesmysql "recipes/internal/recipes/infra/mysql"
	"recipes/internal/recipes/infra/mysql/migrations"
)

func TestRecipePersistence(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	container, err := tcmysql.Run(ctx, "mysql:8.4", tcmysql.WithDatabase("recipes_test"),
		tcmysql.WithUsername("recipes"), tcmysql.WithPassword("recipes-test"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(context.Background())) })
	dsn, err := container.ConnectionString(ctx, "parseTime=true")
	require.NoError(t, err)
	connector := mysql.NewConnector()
	require.NoError(t, connector.Open(dsn, mysql.Config{MaxConnections: 10}))
	t.Cleanup(func() { require.NoError(t, connector.Close()) })
	client := connector.TransactionalClient()
	runner, err := migrator.New(client, slog.Default())
	require.NoError(t, err)
	require.NoError(t, runner.Migrate(ctx, "recipes", migrations.Database(client)...))
	require.NoError(t, runner.Migrate(ctx, "event-outbox", outbox.Migrations(client)...))
	unit := uow.NewUnitOfWork(mysql.NewConnectionPool(client), recipesmysql.NewRepositoryProvider)
	execute := func(fn func(context.Context, *recipesmysql.RepositoryProvider) error) error {
		operationCtx, done := context.WithCancel(ctx)
		defer done()
		return unit.ExecuteWithRepositoryProvider(operationCtx, func(p *recipesmysql.RepositoryProvider) error {
			return fn(operationCtx, p)
		})
	}
	id, first, second, owner := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	recipe, err := domain.NewRecipe(domain.RecipeData{ID: id, Name: "Греча 100%", Servings: 2,
		Ingredients: []domain.Ingredient{{ProductID: first, Quantity: 100}, {ProductID: second, Quantity: 20}},
		Description: "Еда", Instructions: "Варить"})
	require.NoError(t, err)
	require.NoError(
		t,
		execute(func(c context.Context, p *recipesmysql.RepositoryProvider) error {
			if err := p.Recipe(c).Create(recipe); err != nil {
				return err
			}
			return p.RecipeOwnership(c).Create(id, owner)
		}),
	)
	var snapshot *domain.Recipe
	require.NoError(t, execute(func(c context.Context, p *recipesmysql.RepositoryProvider) error {
		var err error
		snapshot, err = p.Recipe(c).GetByID(id)
		return err
	}))
	require.Equal(t, recipe.Data(), snapshot.Data())
	require.NoError(
		t,
		recipe.Revise("Греча по-новому", "", "Смешать", 3, []domain.Ingredient{{ProductID: first, Quantity: 60}}),
	)
	require.NoError(
		t,
		execute(
			func(c context.Context, p *recipesmysql.RepositoryProvider) error { return p.Recipe(c).Update(recipe) },
		),
	)
	require.Len(t, snapshot.Data().Ingredients, 2)
	require.NoError(t, execute(func(c context.Context, p *recipesmysql.RepositoryProvider) error {
		loaded, err := p.Recipe(c).GetByID(id)
		require.NoError(t, err)
		require.Equal(t, recipe.Data(), loaded.Data())
		return nil
	}))
	failure := errors.New("rollback")
	require.ErrorIs(t, execute(func(c context.Context, p *recipesmysql.RepositoryProvider) error {
		if err := recipe.Revise(
			"Не сохранять",
			"",
			"",
			1,
			[]domain.Ingredient{{ProductID: second, Quantity: 1}},
		); err != nil {
			return err
		}
		if err := p.Recipe(c).Update(recipe); err != nil {
			return err
		}
		return failure
	}), failure)
	require.NoError(t, execute(func(c context.Context, p *recipesmysql.RepositoryProvider) error {
		loaded, err := p.Recipe(c).GetByID(id)
		require.NoError(t, err)
		require.Equal(t, "Греча по-новому", loaded.Data().Name)
		require.Equal(t, first, loaded.Data().Ingredients[0].ProductID)
		found, err := p.Recipe(c).List(domain.RecipeFilter{Search: "ГРЕЧ", Limit: 10})
		require.NoError(t, err)
		require.Len(t, found, 1)
		owned, err := p.Recipe(c).ListByOwner(owner, domain.RecipeFilter{Limit: 1})
		require.NoError(t, err)
		require.Len(t, owned, 1)
		foreign, err := p.Recipe(c).ListByOwner(uuid.New(), domain.RecipeFilter{Limit: 1})
		require.NoError(t, err)
		require.Empty(t, foreign)
		recipe = loaded
		return nil
	}))
	recipe.Archive()
	require.NoError(
		t,
		execute(
			func(c context.Context, p *recipesmysql.RepositoryProvider) error { return p.Recipe(c).Update(recipe) },
		),
	)
	require.ErrorIs(t, execute(func(c context.Context, p *recipesmysql.RepositoryProvider) error {
		return p.Recipe(c).Update(snapshot)
	}), domain.ErrRecipeArchived)
	require.NoError(t, execute(func(c context.Context, p *recipesmysql.RepositoryProvider) error {
		found, err := p.Recipe(c).List(domain.RecipeFilter{Limit: 10})
		require.NoError(t, err)
		require.Empty(t, found)
		loaded, err := p.Recipe(c).GetByID(id)
		require.NoError(t, err)
		require.True(t, loaded.Data().Archived)
		require.Equal(t, "Греча по-новому", loaded.Data().Name)
		return nil
	}))
	testRecipeApplication(ctx, t, client, unit, first, second)
}

type fakeCatalog struct{ products map[uuid.UUID]bool }

func (c fakeCatalog) ValidateProduct(_ context.Context, id uuid.UUID) error {
	if !c.products[id] {
		return domain.ErrProductNotFound
	}
	return nil
}

func testRecipeApplication(ctx context.Context, t *testing.T, client mysql.TransactionalClient,
	unit uow.UnitOfWorkWithRepositoryProvider[*recipesmysql.RepositoryProvider], first, second uuid.UUID) {
	t.Helper()
	transport := event.DispatcherFunc(func(context.Context, event.Envelope) error { return nil })
	dispatcher, _, err := outbox.Decorate(transport, uow.NewLockableUnitOfWork(unit), outbox.Config{Transport: "test"})
	require.NoError(t, err)
	service := app.NewService(
		unit,
		eventfactory.NewDispatcherFactory("recipes", dispatcher),
		fakeCatalog{products: map[uuid.UUID]bool{first: true, second: true}},
	)
	owner, stranger := uuid.New(), uuid.New()
	input := domain.RecipeData{
		Name:        "Каша",
		Servings:    2,
		Ingredients: []domain.Ingredient{{ProductID: first, Quantity: 5}},
	}
	recipe, err := service.CreateRecipe(ctx, owner, input)
	require.NoError(t, err)
	_, err = service.GetRecipe(ctx, stranger, recipe.Data().ID)
	require.ErrorIs(t, err, domain.ErrRecipeNotFound)
	listed, err := service.ListRecipes(ctx, stranger, domain.RecipeFilter{})
	require.NoError(t, err)
	require.Empty(t, listed)
	bad := input
	bad.Ingredients = []domain.Ingredient{{ProductID: uuid.New(), Quantity: 1}}
	_, err = service.CreateRecipe(ctx, owner, bad)
	require.ErrorIs(t, err, domain.ErrProductNotFound)
	input.Ingredients = []domain.Ingredient{{ProductID: second, Quantity: 7}}
	require.NoError(t, service.ReviseRecipe(ctx, owner, recipe.Data().ID, input))
	loaded, err := service.GetRecipe(ctx, owner, recipe.Data().ID)
	require.NoError(t, err)
	require.Equal(t, second, loaded.Data().Ingredients[0].ProductID)
	require.NoError(t, service.ArchiveRecipe(ctx, owner, recipe.Data().ID))
	require.NoError(t, service.ArchiveRecipe(ctx, owner, recipe.Data().ID))
	_, err = service.GetRecipe(ctx, owner, recipe.Data().ID)
	require.ErrorIs(t, err, domain.ErrRecipeNotFound)
	var count int
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.events.count", `SELECT COUNT(*) FROM event_outbox`).Scan(&count),
	)
	require.Equal(t, 6, count)
	rollback := errors.New("rollback")
	var rolledBackID uuid.UUID
	opCtx, done := context.WithCancel(ctx)
	err = unit.ExecuteWithRepositoryProvider(opCtx, func(_ *recipesmysql.RepositoryProvider) error {
		created, err := service.CreateRecipe(opCtx, owner, input)
		if err != nil {
			return err
		}
		rolledBackID = created.Data().ID
		return rollback
	})
	done()
	require.ErrorIs(t, err, rollback)
	_, err = service.GetRecipe(ctx, owner, rolledBackID)
	require.ErrorIs(t, err, domain.ErrRecipeNotFound)
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.events.count", `SELECT COUNT(*) FROM event_outbox`).Scan(&count),
	)
	require.Equal(t, 6, count)
	testRecipeHTTP(t, service, first)
}

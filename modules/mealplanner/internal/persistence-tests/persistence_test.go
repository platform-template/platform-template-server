package persistence_tests

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"
	"uuid"

	"mealplanner/internal/mealplanner/domain"
	mealdb "mealplanner/internal/mealplanner/infra/mysql"
	"mealplanner/internal/mealplanner/infra/mysql/migrations"

	"github.com/distributed-programming-2026/go-sdk/pkg/event/inbox"
	"github.com/distributed-programming-2026/go-sdk/pkg/event/outbox"
	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	"github.com/stretchr/testify/require"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
)

func TestMealPersistence(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	container, err := tcmysql.Run(
		ctx,
		"mysql:8.4",
		tcmysql.WithDatabase("meal_test"),
		tcmysql.WithUsername("meal"),
		tcmysql.WithPassword("test-password"),
	)
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
	require.NoError(t, runner.Migrate(ctx, "mealplanner", migrations.Database(client)...))
	require.NoError(t, runner.Migrate(ctx, "event-outbox", outbox.Migrations(client)...))
	require.NoError(t, runner.Migrate(ctx, "event-inbox", inbox.Migrations(client)...))
	unit := uow.NewUnitOfWork(mysql.NewConnectionPool(client), mealdb.NewRepositoryProvider)
	owner, planID, itemID, product, revision := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	plan, err := domain.NewPlan(planID, owner, "Week")
	require.NoError(t, err)
	when := time.Now().UTC().Truncate(time.Microsecond)
	item, err := domain.NewItem(
		domain.ItemData{
			ID:             itemID,
			PlanID:         planID,
			Kind:           domain.Dish,
			RecipeID:       uuid.New(),
			RecipeName:     "Soup",
			Servings:       2,
			ScheduledAt:    when,
			Ingredients:    []domain.Ingredient{{ProductID: product, Quantity: 8}},
			InventoryIDs:   []uuid.UUID{uuid.New()},
			ShoppingListID: uuid.New(),
			Revision:       revision,
		},
	)
	require.NoError(t, err)
	err = unit.ExecuteWithRepositoryProvider(ctx, func(p *mealdb.RepositoryProvider) error {
		if err := p.Plan(ctx).Create(plan); err != nil {
			return err
		}
		return p.Item(ctx).Create(item)
	})
	require.NoError(t, err)
	repo := mealdb.NewRepositoryProvider(client)
	loadedPlan, err := repo.Plan(ctx).Get(owner, planID)
	require.NoError(t, err)
	require.Equal(t, "Week", loadedPlan.Name())
	require.NoError(t, repo.Plan(ctx).Update(loadedPlan))
	require.NoError(t, loadedPlan.Rename("Renamed"))
	require.NoError(t, repo.Plan(ctx).Update(loadedPlan))
	loadedPlan, err = repo.Plan(ctx).Get(owner, planID)
	require.NoError(t, err)
	require.Equal(t, "Renamed", loadedPlan.Name())
	wrongPlan, err := domain.NewPlan(planID, uuid.New(), "Wrong")
	require.NoError(t, err)
	require.ErrorIs(t, repo.Plan(ctx).Update(wrongPlan), domain.ErrPlanNotFound)
	absent, err := repo.Plan(ctx).Get(uuid.New(), planID)
	require.NoError(t, err)
	require.Nil(t, absent)
	plans, err := repo.Plan(ctx).List(owner)
	require.NoError(t, err)
	require.Len(t, plans, 1)
	loaded, version, err := repo.Item(ctx).Get(planID, itemID)
	require.NoError(t, err)
	require.EqualValues(t, 1, version)
	require.Equal(t, item.Data(), loaded.Data())
	changed, err := loaded.ApplyReservation(revision, 1, []domain.Ingredient{{ProductID: product, Quantity: 3}})
	require.NoError(t, err)
	require.True(t, changed)
	require.NoError(t, repo.Item(ctx).Update(loaded, version))
	require.ErrorIs(t, repo.Item(ctx).Update(item, version), domain.ErrConcurrentUpdate)
	restored, newVersion, err := repo.Item(ctx).Get(planID, itemID)
	require.NoError(t, err)
	require.EqualValues(t, 2, newVersion)
	require.Equal(t, domain.Missing, restored.Data().State)
	require.EqualValues(t, 3, restored.Data().Shortages[0].Quantity)
	items, err := repo.Item(ctx).List(planID, when.Add(-time.Hour), when.Add(time.Hour))
	require.NoError(t, err)
	require.Len(t, items, 1)
	failure := errors.New("rollback")
	rollbackItem, err := domain.NewItem(
		domain.ItemData{
			ID:             uuid.New(),
			PlanID:         planID,
			Kind:           domain.Product,
			ScheduledAt:    when,
			Ingredients:    []domain.Ingredient{{ProductID: product, Quantity: 1}},
			InventoryIDs:   []uuid.UUID{uuid.New()},
			ShoppingListID: uuid.New(),
			Revision:       uuid.New(),
		},
	)
	require.NoError(t, err)
	err = unit.ExecuteWithRepositoryProvider(ctx, func(p *mealdb.RepositoryProvider) error {
		if err := p.Item(ctx).Create(rollbackItem); err != nil {
			return err
		}
		return failure
	})
	require.ErrorIs(t, err, failure)
	missing, _, err := repo.Item(ctx).Get(planID, rollbackItem.Data().ID)
	require.NoError(t, err)
	require.Nil(t, missing)
	testPlanApplication(ctx, t, client, unit)
}

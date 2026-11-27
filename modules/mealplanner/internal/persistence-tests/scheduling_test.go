package persistence_tests

import (
	"context"
	"errors"
	"testing"
	"time"
	"uuid"

	mealplannerevents "mealplanner/api/events"
	"mealplanner/internal/mealplanner/app"
	"mealplanner/internal/mealplanner/domain"
	mealdb "mealplanner/internal/mealplanner/infra/mysql"

	"github.com/distributed-programming-2026/go-sdk/pkg/event"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	"github.com/stretchr/testify/require"
)

type fakeInventory struct{ err error }

const testRecipeName = "Soup"

func (f fakeInventory) CheckOwners(context.Context, uuid.UUID, []uuid.UUID) error { return f.err }
func (f fakeInventory) ValidateProduct(context.Context, uuid.UUID) error          { return f.err }

type fakeShopping struct{ err error }

func (f fakeShopping) CheckOwner(context.Context, uuid.UUID, uuid.UUID) error { return f.err }

type fakeRecipes struct {
	snapshot domain.RecipeSnapshot
	err      error
}

func (f fakeRecipes) Snapshot(context.Context, uuid.UUID, uuid.UUID, int64) (domain.RecipeSnapshot, error) {
	return f.snapshot, f.err
}

func testScheduling(
	ctx context.Context,
	t *testing.T,
	client mysql.TransactionalClient,
	unit uow.UnitOfWorkWithRepositoryProvider[*mealdb.RepositoryProvider],
	base *app.Service,
	owner, planID uuid.UUID,
) {
	t.Helper()
	recipe, product := uuid.New(), uuid.New()
	ingredients := []domain.Ingredient{{ProductID: product, Quantity: 5}}
	deps := app.PlanningDependencies{
		Inventory: fakeInventory{},
		Shopping:  fakeShopping{},
		Recipes: fakeRecipes{
			snapshot: domain.RecipeSnapshot{
				RecipeID:    recipe,
				Name:        testRecipeName,
				Servings:    2,
				Ingredients: ingredients,
			},
		},
	}
	service := app.NewPlanningService(base, deps)
	input := app.ScheduleInput{
		PlanID:         planID,
		ShoppingListID: uuid.New(),
		InventoryIDs:   []uuid.UUID{uuid.New()},
		ScheduledAt:    time.Now(),
		Kind:           domain.Dish,
		RecipeID:       recipe,
		Servings:       2,
	}
	item, err := service.Schedule(ctx, owner, input)
	require.NoError(t, err)
	require.Equal(t, domain.Planning, item.Data().State)
	ingredients[0].Quantity = 100
	require.EqualValues(t, 5, item.Data().Ingredients[0].Quantity)
	stored, _, err := mealdb.NewRepositoryProvider(client).Item(ctx).Get(planID, item.Data().ID)
	require.NoError(t, err)
	require.EqualValues(t, 5, stored.Data().Ingredients[0].Quantity)
	rows, err := client.QueryContext(
		ctx,
		"test.schedule.events",
		`SELECT payload FROM event_outbox ORDER BY id DESC LIMIT 2`,
	)
	require.NoError(t, err)
	types := map[string]bool{}
	for rows.Next() {
		var raw []byte
		require.NoError(t, rows.Scan(&raw))
		envelope, err := event.Unmarshal(raw)
		require.NoError(t, err)
		types[envelope.Type()] = true
		if envelope.Type() == "MealPlanningRequested" {
			var payload mealplannerevents.MealPlanningRequested
			require.NoError(t, envelope.UnmarshalPayload(&payload))
			require.Equal(t, item.Data().ID.String(), payload.ItemId)
			require.Equal(t, item.Data().Revision.String(), payload.Revision)
			require.EqualValues(t, 5, payload.Ingredients[0].Quantity)
		}
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.True(t, types["MealPlanningRequested"])
	require.True(t, types["AuditRecordRequested"])
	var countBefore, countAfter int
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.outbox.count", `SELECT COUNT(*) FROM event_outbox`).Scan(&countBefore),
	)
	failure := errors.New("rollback")
	operationCtx, done := context.WithCancel(ctx)
	defer done()
	var rolled uuid.UUID
	err = unit.ExecuteWithRepositoryProvider(operationCtx, func(_ *mealdb.RepositoryProvider) error {
		item, err := service.Schedule(operationCtx, owner, input)
		if err != nil {
			return err
		}
		rolled = item.Data().ID
		return failure
	})
	require.ErrorIs(t, err, failure)
	missing, _, err := mealdb.NewRepositoryProvider(client).Item(ctx).Get(planID, rolled)
	require.NoError(t, err)
	require.Nil(t, missing)
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.outbox.count", `SELECT COUNT(*) FROM event_outbox`).Scan(&countAfter),
	)
	require.Equal(t, countBefore, countAfter)
	deps.Shopping = fakeShopping{err: errors.New("denied")}
	denied := app.NewPlanningService(base, deps)
	_, err = denied.Schedule(ctx, owner, input)
	require.Error(t, err)
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.outbox.count", `SELECT COUNT(*) FROM event_outbox`).Scan(&countAfter),
	)
	require.Equal(t, countBefore, countAfter)
	testAllocationInbox(ctx, t, client, unit, base, owner, item)
	testCancellationInbox(ctx, t, client, unit, base, owner, item)
	testCompletionInbox(ctx, t, client, unit, base, owner, planID)
}

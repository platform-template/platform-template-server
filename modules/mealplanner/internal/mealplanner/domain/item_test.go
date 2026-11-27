package domain

import (
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/require"
)

func TestItemReservationLifecycle(t *testing.T) {
	product, revision := uuid.New(), uuid.New()
	first, second := uuid.New(), uuid.New()
	data := ItemData{
		ID: uuid.New(), PlanID: uuid.New(), Kind: Dish, RecipeID: uuid.New(), RecipeName: "Soup", Servings: 2,
		ScheduledAt: time.Now(), Ingredients: []Ingredient{{ProductID: product, Quantity: 10}},
		InventoryIDs: []uuid.UUID{first, second}, ShoppingListID: uuid.New(), Revision: revision,
	}
	item, err := NewItem(data)
	require.NoError(t, err)
	require.Equal(t, Planning, item.Data().State)
	data.Ingredients[0].Quantity = 100
	returned := item.Data()
	returned.InventoryIDs[0] = uuid.Nil()
	require.EqualValues(t, 10, item.Data().Ingredients[0].Quantity)
	require.NotEqual(t, uuid.Nil(), item.Data().InventoryIDs[0])
	changed, err := item.ApplyReservation(uuid.New(), 1, nil)
	require.NoError(t, err)
	require.False(t, changed)
	changed, err = item.ApplyReservation(revision, 1, []Ingredient{{ProductID: product, Quantity: 4}})
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, Missing, item.Data().State)
	require.ErrorIs(t, item.RequestCompletion(), ErrInvalidTransition)
	changed, err = item.ApplyReservation(revision, 2, nil)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, Ready, item.Data().State)
	changed, err = item.ApplyReservation(revision, 1, []Ingredient{{ProductID: product, Quantity: 4}})
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, Ready, item.Data().State)
	require.NoError(t, item.RequestCompletion())
	require.False(t, item.ConfirmCompletion(uuid.New()))
	require.True(t, item.ConfirmCompletion(revision))
	require.Equal(t, Completed, item.Data().State)
	require.ErrorIs(t, item.RequestCancellation(), ErrInvalidTransition)
}

func TestItemValidationAndCancellation(t *testing.T) {
	product, revision := uuid.New(), uuid.New()
	data := ItemData{ID: uuid.New(), PlanID: uuid.New(), Kind: Product, ScheduledAt: time.Now(),
		Ingredients: []Ingredient{{ProductID: product, Quantity: 1}}, InventoryIDs: []uuid.UUID{uuid.New()},
		ShoppingListID: uuid.New(), Revision: revision}
	item, err := NewItem(data)
	require.NoError(t, err)
	_, err = item.ApplyReservation(revision, 1, []Ingredient{{ProductID: uuid.New(), Quantity: 1}})
	require.ErrorIs(t, err, ErrInvalidItem)
	_, err = item.ApplyReservation(revision, 1, []Ingredient{{ProductID: product, Quantity: 2}})
	require.ErrorIs(t, err, ErrInvalidItem)
	require.NoError(t, item.RequestCancellation())
	changed, err := item.ApplyReservation(revision, 1, nil)
	require.NoError(t, err)
	require.False(t, changed)
	changed, err = item.ConfirmCancellation(uuid.New(), InventoryParticipant)
	require.NoError(t, err)
	require.False(t, changed)
	changed, err = item.ConfirmCancellation(revision, InventoryParticipant)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, Cancelling, item.Data().State)
	changed, err = item.ConfirmCancellation(revision, ShoppingParticipant)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, Cancelled, item.Data().State)
	data.InventoryIDs = append(data.InventoryIDs, data.InventoryIDs[0])
	_, err = NewItem(data)
	require.ErrorIs(t, err, ErrInvalidItem)
}

func TestItemRetriesAfterCompensatingPartialAllocation(t *testing.T) {
	product, inventory, revision := uuid.New(), uuid.New(), uuid.New()
	item, err := NewItem(
		ItemData{
			ID:             uuid.New(),
			PlanID:         uuid.New(),
			Kind:           Product,
			ScheduledAt:    time.Now(),
			Ingredients:    []Ingredient{{ProductID: product, Quantity: 2}},
			InventoryIDs:   []uuid.UUID{inventory},
			ShoppingListID: uuid.New(),
			Revision:       revision,
		},
	)
	require.NoError(t, err)
	changed, err := item.ApplyAllocation(
		revision,
		1,
		[]AllocationLine{{InventoryID: inventory, ReservationID: uuid.New(), ProductID: product, Quantity: 1}},
		[]Ingredient{{ProductID: product, Quantity: 1}},
	)
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, Missing, item.Data().State)

	require.NoError(t, item.RequestRetry())
	_, err = item.ConfirmCancellation(revision, InventoryParticipant)
	require.NoError(t, err)
	_, err = item.ConfirmCancellation(revision, ShoppingParticipant)
	require.NoError(t, err)
	require.Equal(t, Cancelling, item.Data().State)
	require.True(t, item.Data().RetryRequested)

	nextRevision := uuid.New()
	require.True(t, item.RestartAfterCancellation(nextRevision))
	retried := item.Data()
	require.Equal(t, Planning, retried.State)
	require.Equal(t, nextRevision, retried.Revision)
	require.Zero(t, retried.ReservationVersion)
	require.Empty(t, retried.Allocations)
	require.Empty(t, retried.Shortages)

	require.NoError(t, item.RequestCancellation())
	require.Equal(t, Cancelling, item.Data().State)
}

func TestItemUserCancellationWinsOverRetry(t *testing.T) {
	product, revision := uuid.New(), uuid.New()
	item, err := NewItem(
		ItemData{
			ID:             uuid.New(),
			PlanID:         uuid.New(),
			Kind:           Product,
			ScheduledAt:    time.Now(),
			Ingredients:    []Ingredient{{ProductID: product, Quantity: 1}},
			InventoryIDs:   []uuid.UUID{uuid.New()},
			ShoppingListID: uuid.New(),
			Revision:       revision,
		},
	)
	require.NoError(t, err)
	changed, err := item.ApplyReservation(revision, 1, []Ingredient{{ProductID: product, Quantity: 1}})
	require.NoError(t, err)
	require.True(t, changed)
	require.NoError(t, item.RequestRetry())
	require.True(t, item.CancelRetry())
	_, err = item.ConfirmCancellation(revision, InventoryParticipant)
	require.NoError(t, err)
	_, err = item.ConfirmCancellation(revision, ShoppingParticipant)
	require.NoError(t, err)
	require.Equal(t, Cancelled, item.Data().State)
}

func TestPlanRequiresOwnerAndName(t *testing.T) {
	_, err := NewPlan(uuid.New(), uuid.Nil(), "Plan")
	require.ErrorIs(t, err, ErrInvalidPlan)
	plan, err := NewPlan(uuid.New(), uuid.New(), "Plan")
	require.NoError(t, err)
	require.ErrorIs(t, plan.Rename("  "), ErrInvalidPlan)
	require.NoError(t, plan.Rename("Week"))
	require.Equal(t, "Week", plan.Name())
}

func TestAllocationMustAccountForAllIngredients(t *testing.T) {
	product, inventory, revision := uuid.New(), uuid.New(), uuid.New()
	item, err := NewItem(
		ItemData{
			ID:             uuid.New(),
			PlanID:         uuid.New(),
			Kind:           Product,
			ScheduledAt:    time.Now(),
			Ingredients:    []Ingredient{{ProductID: product, Quantity: 5}},
			InventoryIDs:   []uuid.UUID{inventory},
			ShoppingListID: uuid.New(),
			Revision:       revision,
		},
	)
	require.NoError(t, err)
	_, err = item.ApplyAllocation(revision, 1, nil, nil)
	require.ErrorIs(t, err, ErrInvalidItem)
	lines := []AllocationLine{{InventoryID: inventory, ReservationID: uuid.New(), ProductID: product, Quantity: 3}}
	_, err = item.ApplyAllocation(revision, 1, lines, []Ingredient{{ProductID: product, Quantity: 1}})
	require.ErrorIs(t, err, ErrInvalidItem)
	changed, err := item.ApplyAllocation(revision, 1, lines, []Ingredient{{ProductID: product, Quantity: 2}})
	require.NoError(t, err)
	require.True(t, changed)
	lines[0].Quantity = 99
	require.EqualValues(t, 3, item.Data().Allocations[0].Quantity)
	restored, err := RestoreItem(item.Data())
	require.NoError(t, err)
	require.Equal(t, item.Data(), restored.Data())
}

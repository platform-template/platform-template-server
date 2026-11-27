package domain

import (
	"testing"
	"uuid"

	"github.com/stretchr/testify/require"
)

func TestAutomaticDemandCancellationKeepsPurchaseAndSource(t *testing.T) {
	source := Source{MealPlanID: uuid.New(), MealItemID: uuid.New(), Revision: uuid.New()}
	data := DemandData{ID: uuid.New(), ListID: uuid.New(), ProductID: uuid.New(), Required: 10, Source: &source}
	demand, err := NewDemand(data)
	require.NoError(t, err)
	source.Revision = uuid.New()
	require.NotEqual(t, source.Revision, demand.Data().Source.Revision)
	purchase, err := NewPurchase(
		PurchaseData{
			ID:          uuid.New(),
			DemandID:    data.ID,
			ProductID:   data.ProductID,
			InventoryID: uuid.New(),
			Quantity:    4,
		},
	)
	require.NoError(t, err)
	changed, err := demand.ApplyPurchase(purchase)
	require.NoError(t, err)
	require.True(t, changed)
	require.EqualValues(t, 6, demand.Remaining())
	changed, err = demand.ApplyPurchase(purchase)
	require.NoError(t, err)
	require.False(t, changed)
	require.EqualValues(t, 4, demand.Data().Purchased)
	require.False(t, demand.CancelSource(source))
	original := *demand.Data().Source
	require.True(t, demand.CancelSource(original))
	require.False(t, demand.CancelSource(original))
	require.EqualValues(t, 0, demand.Remaining())
	require.EqualValues(t, 4, demand.Data().Purchased)
	require.ErrorIs(t, demand.RemoveManual(), ErrAutomaticDemand)
	require.ErrorIs(t, demand.ChangeManualQuantity(20), ErrAutomaticDemand)
}

func TestPurchaseConfirmationAfterCancellationAndOverpurchase(t *testing.T) {
	source := Source{MealPlanID: uuid.New(), MealItemID: uuid.New(), Revision: uuid.New()}
	demand, err := NewDemand(
		DemandData{ID: uuid.New(), ListID: uuid.New(), ProductID: uuid.New(), Required: 3, Source: &source},
	)
	require.NoError(t, err)
	data := demand.Data()
	purchase, err := NewPurchase(
		PurchaseData{
			ID:          uuid.New(),
			DemandID:    data.ID,
			ProductID:   data.ProductID,
			InventoryID: uuid.New(),
			Quantity:    5,
		},
	)
	require.NoError(t, err)
	require.True(t, demand.CancelSource(source))
	changed, err := demand.ApplyPurchase(purchase)
	require.NoError(t, err)
	require.True(t, changed)
	require.EqualValues(t, 5, demand.Data().Purchased)
	require.Zero(t, demand.Remaining())
	require.True(t, purchase.Data().Confirmed)
}

func TestManualDemandDoesNotShareAutomaticCancellation(t *testing.T) {
	demand, err := NewDemand(DemandData{ID: uuid.New(), ListID: uuid.New(), ProductID: uuid.New(), Required: 6})
	require.NoError(t, err)
	require.False(t, demand.CancelSource(Source{MealPlanID: uuid.New(), MealItemID: uuid.New(), Revision: uuid.New()}))
	require.NoError(t, demand.ChangeManualQuantity(9))
	require.EqualValues(t, 9, demand.Remaining())
	require.NoError(t, demand.RemoveManual())
	require.Zero(t, demand.Remaining())
}

func TestPurchaseCannotApplyToDifferentDemand(t *testing.T) {
	demand, err := NewDemand(DemandData{ID: uuid.New(), ListID: uuid.New(), ProductID: uuid.New(), Required: 1})
	require.NoError(t, err)
	purchase, err := NewPurchase(
		PurchaseData{
			ID:          uuid.New(),
			DemandID:    uuid.New(),
			ProductID:   demand.Data().ProductID,
			InventoryID: uuid.New(),
			Quantity:    1,
		},
	)
	require.NoError(t, err)
	_, err = demand.ApplyPurchase(purchase)
	require.ErrorIs(t, err, ErrInvalidPurchase)
	require.False(t, purchase.Data().Confirmed)
}

package persistence_tests

import (
	"errors"
	"sync"
	"testing"
	"uuid"

	"github.com/stretchr/testify/require"

	"inventory/internal/inventory/domain"
)

func TestProductSearch(t *testing.T) {
	svc := newService()
	for _, name := range []string{"Молоко", "Молоко кокосовое", "Соль 100%", "Соль _морская", "Соль !острая"} {
		_, err := svc.CreateProduct(t.Context(), name, domain.UnitGrams)
		require.NoError(t, err)
	}
	for _, test := range []struct {
		search string
		count  int
	}{
		{
			search: "мОЛ",
			count:  2,
		},
		{
			search: "100%",
			count:  1,
		},
		{
			search: "_мор",
			count:  1,
		},
		{
			search: "!ост",
			count:  1,
		},
		{
			search: "несуществующее",
			count:  0,
		},
	} {
		products, err := svc.ListProducts(t.Context(), domain.ProductFilter{Search: test.search})
		require.NoError(t, err)
		require.NotNil(t, products)
		require.Len(t, products, test.count)
	}
	first, err := svc.ListProducts(t.Context(), domain.ProductFilter{
		Search: "мол",
		Limit:  1,
	})
	require.NoError(t, err)
	second, err := svc.ListProducts(t.Context(), domain.ProductFilter{
		Search: "мол",
		Limit:  1,
		Offset: 1,
	})
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.Len(t, second, 1)
	require.NotEqual(t, first[0].ID(), second[0].ID())
	require.Equal(t, "Молоко", first[0].Name())
	require.Equal(t, "Молоко кокосовое", second[0].Name())
}

func TestConsumeLifecycle(t *testing.T) {
	svc := newService()
	first := addProduct(t, svc, 10)
	second := addProduct(t, svc, 20)
	id := uuid.New()
	items := []domain.ReservationItem{
		{
			ProductID: first,
			Quantity:  7,
		},
		{
			ProductID: second,
			Quantity:  4,
		},
	}
	require.NoError(t, svc.Reserve(t.Context(), testInventoryID, id, items))
	require.NoError(t, svc.Consume(t.Context(), testInventoryID, id))
	require.NoError(t, svc.Consume(t.Context(), testInventoryID, id))
	require.ErrorIs(t, svc.Release(t.Context(), testInventoryID, id), domain.ErrReservationConsumed)
	require.ErrorIs(t, svc.Reserve(t.Context(), testInventoryID, id, items), domain.ErrReservationConsumed)
	stock, err := svc.GetStock(t.Context(), testInventoryID, first)
	require.NoError(t, err)
	require.EqualValues(t, 3, stock.Quantity)
	require.Zero(t, stock.ReservedQuantity)
	stock, err = svc.GetStock(t.Context(), testInventoryID, second)
	require.NoError(t, err)
	require.EqualValues(t, 16, stock.Quantity)
	require.Zero(t, stock.ReservedQuantity)
	require.ErrorIs(t, svc.Consume(t.Context(), testInventoryID, uuid.New()), domain.ErrReservationNotFound)
	cancelled := uuid.New()
	require.NoError(t, svc.Release(t.Context(), testInventoryID, cancelled))
	require.ErrorIs(t, svc.Consume(t.Context(), testInventoryID, cancelled), domain.ErrReservationCancelled)
}

func TestConcurrentConsume(t *testing.T) {
	for _, release := range []bool{false, true} {
		t.Run(map[bool]string{false: "consume-consume", true: "consume-release"}[release], func(t *testing.T) {
			svc := newService()
			productID := addProduct(t, svc, 10)
			id := uuid.New()
			items := []domain.ReservationItem{
				{
					ProductID: productID,
					Quantity:  7,
				},
			}
			require.NoError(t, svc.Reserve(t.Context(), testInventoryID, id, items))
			results := make(chan error, 2)
			var group sync.WaitGroup
			group.Add(2)
			go func() { defer group.Done(); results <- svc.Consume(testOperationContext(), testInventoryID, id) }()
			go func() {
				defer group.Done()
				if release {
					results <- svc.Release(testOperationContext(), testInventoryID, id)
					return
				}
				results <- svc.Consume(testOperationContext(), testInventoryID, id)
			}()
			group.Wait()
			close(results)
			for err := range results {
				if err != nil {
					require.True(
						t,
						errors.Is(err, domain.ErrReservationCancelled) || errors.Is(err, domain.ErrReservationConsumed),
					)
				}
			}
			stock, err := svc.GetStock(t.Context(), testInventoryID, productID)
			require.NoError(t, err)
			require.Zero(t, stock.ReservedQuantity)
			if !release {
				require.EqualValues(t, 3, stock.Quantity)
			} else {
				require.Contains(t, []int64{3, 10}, stock.Quantity)
			}
		})
	}
}

func TestConsumeRollsBackAllStocks(t *testing.T) {
	svc := newService()
	first := addProduct(t, svc, 10)
	second := addProduct(t, svc, 10)
	if first.String() > second.String() {
		first, second = second, first
	}
	id := uuid.New()
	items := []domain.ReservationItem{
		{
			ProductID: first,
			Quantity:  3,
		},
		{
			ProductID: second,
			Quantity:  4,
		},
	}
	require.NoError(t, svc.Reserve(t.Context(), testInventoryID, id, items))
	_, err := client.ExecContext(
		t.Context(),
		"test.corrupt_reservation_item",
		`UPDATE reservation_items SET quantity = 5 WHERE reservation_id = ? AND product_id = ?`,
		id[:],
		second[:],
	)
	require.NoError(t, err)
	require.ErrorIs(t, svc.Consume(t.Context(), testInventoryID, id), domain.ErrInvalidStock)
	stock, err := svc.GetStock(t.Context(), testInventoryID, first)
	require.NoError(t, err)
	require.EqualValues(t, 10, stock.Quantity)
	require.EqualValues(t, 3, stock.ReservedQuantity)
}

func TestConcurrentAddStock(t *testing.T) {
	svc := newService()
	productID := addProduct(t, svc, 1)
	results := make(chan error, 8)
	for range 8 {
		go func() { results <- svc.AddStock(testOperationContext(), testInventoryID, productID, 2) }()
	}
	for range 8 {
		require.NoError(t, <-results)
	}
	stock, err := svc.GetStock(t.Context(), testInventoryID, productID)
	require.NoError(t, err)
	require.EqualValues(t, 17, stock.Quantity)
}

func TestConcurrentReserveAndConsume(t *testing.T) {
	svc := newService()
	productID := addProduct(t, svc, 10)
	id := uuid.New()
	items := []domain.ReservationItem{
		{
			ProductID: productID,
			Quantity:  7,
		},
	}
	require.NoError(t, svc.Reserve(t.Context(), testInventoryID, id, items))
	reserveResult := make(chan error, 1)
	consumeResult := make(chan error, 1)
	go func() { reserveResult <- svc.Reserve(testOperationContext(), testInventoryID, id, items) }()
	go func() { consumeResult <- svc.Consume(testOperationContext(), testInventoryID, id) }()
	err := <-reserveResult
	if err != nil {
		require.ErrorIs(t, err, domain.ErrReservationConsumed)
	}
	require.NoError(t, <-consumeResult)
	stock, err := svc.GetStock(t.Context(), testInventoryID, productID)
	require.NoError(t, err)
	require.EqualValues(t, 3, stock.Quantity)
	require.Zero(t, stock.ReservedQuantity)
}

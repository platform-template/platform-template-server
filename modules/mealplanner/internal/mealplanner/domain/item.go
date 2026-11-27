package domain

import (
	"errors"
	"sort"
	"strings"
	"time"
	"uuid"
)

var (
	ErrInvalidItem       = errors.New("invalid meal item")
	ErrInvalidTransition = errors.New("invalid meal item transition")
)

type ItemKind string

const (
	Dish    ItemKind = "dish"
	Product ItemKind = "product"
)

type ItemState string

const (
	Planning   ItemState = "planning"
	Missing    ItemState = "missing"
	Ready      ItemState = "ready"
	Completing ItemState = "completing"
	Completed  ItemState = "completed"
	Cancelling ItemState = "cancelling"
	Cancelled  ItemState = "cancelled"
	Failed     ItemState = "failed"
)

type Ingredient struct {
	ProductID uuid.UUID
	Quantity  int64
}

type RecipeSnapshot struct {
	RecipeID     uuid.UUID
	Name         string
	Description  string
	Instructions string
	Servings     int64
	Ingredients  []Ingredient
}

type ItemData struct {
	ID                 uuid.UUID
	PlanID             uuid.UUID
	Kind               ItemKind
	RecipeID           uuid.UUID
	RecipeName         string
	RecipeDescription  string
	RecipeInstructions string
	Servings           int64
	ScheduledAt        time.Time
	Ingredients        []Ingredient
	InventoryIDs       []uuid.UUID
	ShoppingListID     uuid.UUID
	Revision           uuid.UUID
	ReservationVersion uint64
	State              ItemState
	Shortages          []Ingredient
}

type Item struct{ data ItemData }

func NewItem(data ItemData) (*Item, error) {
	if data.ID == uuid.Nil() || data.PlanID == uuid.Nil() || data.Revision == uuid.Nil() ||
		data.ShoppingListID == uuid.Nil() || data.ScheduledAt.IsZero() || len(data.InventoryIDs) == 0 ||
		!validIngredients(data.Ingredients) {
		return nil, ErrInvalidItem
	}
	switch data.Kind {
	case Dish:
		if data.RecipeID == uuid.Nil() || strings.TrimSpace(data.RecipeName) == "" || data.Servings <= 0 {
			return nil, ErrInvalidItem
		}

	case Product:
		if data.RecipeID != uuid.Nil() || data.RecipeName != "" || data.Servings != 0 || len(data.Ingredients) != 1 {
			return nil, ErrInvalidItem
		}

	default:
		return nil, ErrInvalidItem
	}
	seen := make(map[uuid.UUID]bool, len(data.InventoryIDs))
	for _, id := range data.InventoryIDs {
		if id == uuid.Nil() || seen[id] {
			return nil, ErrInvalidItem
		}
		seen[id] = true
	}
	data.Ingredients = append([]Ingredient(nil), data.Ingredients...)
	data.InventoryIDs = append([]uuid.UUID(nil), data.InventoryIDs...)
	// Distribution order is stable regardless of client input order.
	sort.Slice(
		data.InventoryIDs,
		func(i, j int) bool { return data.InventoryIDs[i].String() < data.InventoryIDs[j].String() },
	)
	data.Shortages = nil
	data.State = Planning
	data.ReservationVersion = 0
	data.ScheduledAt = data.ScheduledAt.UTC()
	return &Item{data: data}, nil
}

// RestoreItem validates persisted data before reconstructing its operation state.
func RestoreItem(data ItemData) (*Item, error) {
	item, err := NewItem(data)
	if err != nil {
		return nil, err
	}
	switch data.State {
	case Planning:
		if data.ReservationVersion != 0 || len(data.Shortages) != 0 {
			return nil, ErrInvalidItem
		}
	case Missing:
		if data.ReservationVersion == 0 || len(data.Shortages) == 0 {
			return nil, ErrInvalidItem
		}
	case Ready, Completing, Completed:
		if data.ReservationVersion == 0 || len(data.Shortages) != 0 {
			return nil, ErrInvalidItem
		}
	case Cancelling, Cancelled, Failed:
	default:
		return nil, ErrInvalidItem
	}
	if len(data.Shortages) > 0 {
		valid, err := item.ApplyReservation(data.Revision, 1, data.Shortages)
		if err != nil || !valid {
			return nil, ErrInvalidItem
		}
	}
	item.data.State = data.State
	item.data.ReservationVersion = data.ReservationVersion
	item.data.Shortages = append([]Ingredient(nil), data.Shortages...)
	return item, nil
}

func validIngredients(items []Ingredient) bool {
	if len(items) == 0 {
		return false
	}
	seen := make(map[uuid.UUID]bool, len(items))
	for _, item := range items {
		if item.ProductID == uuid.Nil() || item.Quantity <= 0 || seen[item.ProductID] {
			return false
		}
		seen[item.ProductID] = true
	}
	return true
}

func (i *Item) Data() ItemData {
	data := i.data
	data.Ingredients = append([]Ingredient(nil), data.Ingredients...)
	data.InventoryIDs = append([]uuid.UUID(nil), data.InventoryIDs...)
	data.Shortages = append([]Ingredient(nil), data.Shortages...)
	return data
}

// ApplyReservation ignores results from an earlier revision or a terminal operation.
func (i *Item) ApplyReservation(revision uuid.UUID, version uint64, shortages []Ingredient) (bool, error) {
	if revision != i.data.Revision || version <= i.data.ReservationVersion {
		return false, nil
	}
	if i.data.State != Planning && i.data.State != Missing && i.data.State != Ready {
		return false, nil
	}
	if len(shortages) > 0 && !validIngredients(shortages) {
		return false, ErrInvalidItem
	}
	required := make(map[uuid.UUID]int64, len(i.data.Ingredients))
	for _, ingredient := range i.data.Ingredients {
		required[ingredient.ProductID] = ingredient.Quantity
	}
	for _, shortage := range shortages {
		if quantity, ok := required[shortage.ProductID]; !ok || shortage.Quantity > quantity {
			return false, ErrInvalidItem
		}
	}
	i.data.Shortages = append([]Ingredient(nil), shortages...)
	i.data.ReservationVersion = version
	if len(shortages) == 0 {
		i.data.State = Ready
	} else {
		i.data.State = Missing
	}
	return true, nil
}

func (i *Item) RequestCompletion() error {
	if i.data.State != Ready {
		return ErrInvalidTransition
	}
	i.data.State = Completing
	return nil
}

func (i *Item) ConfirmCompletion(revision uuid.UUID) bool {
	if revision != i.data.Revision || i.data.State != Completing {
		return false
	}
	i.data.State = Completed
	return true
}

func (i *Item) RequestCancellation() error {
	switch i.data.State {
	case Planning, Missing, Ready, Failed:
		i.data.State = Cancelling
		return nil
	default:
		return ErrInvalidTransition
	}
}

func (i *Item) ConfirmCancellation(revision uuid.UUID) bool {
	if revision != i.data.Revision || i.data.State != Cancelling {
		return false
	}
	i.data.State = Cancelled
	return true
}

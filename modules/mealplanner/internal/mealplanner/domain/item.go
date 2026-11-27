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
	ErrItemNotFound      = errors.New("meal item not found")
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

type AllocationLine struct {
	InventoryID   uuid.UUID
	ReservationID uuid.UUID
	ProductID     uuid.UUID
	Quantity      int64
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
	ID                             uuid.UUID
	PlanID                         uuid.UUID
	Kind                           ItemKind
	RecipeID                       uuid.UUID
	RecipeName                     string
	RecipeDescription              string
	RecipeInstructions             string
	Servings                       int64
	ScheduledAt                    time.Time
	Ingredients                    []Ingredient
	InventoryIDs                   []uuid.UUID
	ShoppingListID                 uuid.UUID
	Revision                       uuid.UUID
	ReservationVersion             uint64
	State                          ItemState
	Shortages                      []Ingredient
	Allocations                    []AllocationLine
	InventoryCancellationConfirmed bool
	ShoppingCancellationConfirmed  bool
	RetryRequested                 bool
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
	data.Allocations = nil
	data.State = Planning
	data.InventoryCancellationConfirmed = false
	data.ShoppingCancellationConfirmed = false
	data.RetryRequested = false
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
	case Cancelling:
		if data.InventoryCancellationConfirmed && data.ShoppingCancellationConfirmed {
			return nil, ErrInvalidItem
		}
	case Cancelled:
		if !data.InventoryCancellationConfirmed || !data.ShoppingCancellationConfirmed || data.RetryRequested {
			return nil, ErrInvalidItem
		}
	case Failed:
	default:
		return nil, ErrInvalidItem
	}
	if len(data.Shortages) > 0 {
		valid, err := item.ApplyReservation(data.Revision, 1, data.Shortages)
		if err != nil || !valid {
			return nil, ErrInvalidItem
		}
	}
	if len(data.Allocations) > 0 {
		check, err := NewItem(data)
		if err != nil {
			return nil, err
		}
		changed, err := check.ApplyAllocation(data.Revision, 1, data.Allocations, data.Shortages)
		if err != nil || !changed {
			return nil, ErrInvalidItem
		}
	}
	item.data.Allocations = append([]AllocationLine(nil), data.Allocations...)
	item.data.InventoryCancellationConfirmed = data.InventoryCancellationConfirmed
	item.data.ShoppingCancellationConfirmed = data.ShoppingCancellationConfirmed
	item.data.RetryRequested = data.RetryRequested
	item.data.State = data.State
	item.data.ReservationVersion = data.ReservationVersion
	item.data.Shortages = append([]Ingredient(nil), data.Shortages...)
	data.Allocations = append([]AllocationLine(nil), data.Allocations...)
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

// RequestRetry compensates the current partial allocation before creating a new
// revision. Existing reservations stay terminal and cannot leak into the retry.
func (i *Item) RequestRetry() error {
	if i.data.State != Missing {
		return ErrInvalidTransition
	}
	i.data.State = Cancelling
	i.data.RetryRequested = true
	i.data.InventoryCancellationConfirmed = false
	i.data.ShoppingCancellationConfirmed = false
	return nil
}

// CancelRetry gives an explicit user cancellation precedence over a purchase
// notification that was already in flight.
func (i *Item) CancelRetry() bool {
	if i.data.State != Cancelling || !i.data.RetryRequested {
		return false
	}
	i.data.RetryRequested = false
	return true
}

func (i *Item) RestartAfterCancellation(nextRevision uuid.UUID) bool {
	if nextRevision == uuid.Nil() || i.data.State != Cancelling || !i.data.RetryRequested ||
		!i.data.InventoryCancellationConfirmed || !i.data.ShoppingCancellationConfirmed {
		return false
	}
	i.data.Revision = nextRevision
	i.data.State = Planning
	i.data.ReservationVersion = 0
	i.data.Shortages = nil
	i.data.Allocations = nil
	i.data.InventoryCancellationConfirmed = false
	i.data.ShoppingCancellationConfirmed = false
	i.data.RetryRequested = false
	return true
}

type CancellationParticipant string

const (
	InventoryParticipant CancellationParticipant = "inventory"
	ShoppingParticipant  CancellationParticipant = "shopping"
)

func (i *Item) ConfirmCancellation(revision uuid.UUID, participant CancellationParticipant) (bool, error) {
	if revision != i.data.Revision || i.data.State == Cancelled {
		return false, nil
	}
	if i.data.State != Cancelling {
		return false, ErrInvalidTransition
	}
	switch participant {
	case InventoryParticipant:
		if i.data.InventoryCancellationConfirmed {
			return false, nil
		}
		i.data.InventoryCancellationConfirmed = true
	case ShoppingParticipant:
		if i.data.ShoppingCancellationConfirmed {
			return false, nil
		}
		i.data.ShoppingCancellationConfirmed = true
	default:
		return false, ErrInvalidTransition
	}
	if i.data.InventoryCancellationConfirmed && i.data.ShoppingCancellationConfirmed && !i.data.RetryRequested {
		i.data.State = Cancelled
	}
	return true, nil
}

// ApplyAllocation verifies that reserved and missing quantities account for the snapshot.
func (i *Item) ApplyAllocation(
	revision uuid.UUID,
	version uint64,
	allocations []AllocationLine,
	shortages []Ingredient,
) (bool, error) {
	if revision != i.data.Revision || version <= i.data.ReservationVersion {
		return false, nil
	}
	if i.data.State != Planning && i.data.State != Missing && i.data.State != Ready {
		return false, nil
	}
	remaining := make(map[uuid.UUID]int64, len(i.data.Ingredients))
	for _, ingredient := range i.data.Ingredients {
		remaining[ingredient.ProductID] = ingredient.Quantity
	}
	allowed := make(map[uuid.UUID]bool, len(i.data.InventoryIDs))
	for _, inventoryID := range i.data.InventoryIDs {
		allowed[inventoryID] = true
	}
	type pair struct{ inventory, product uuid.UUID }
	seen := make(map[pair]bool)
	reservationByInventory := make(map[uuid.UUID]uuid.UUID)
	for _, line := range allocations {
		key := pair{line.InventoryID, line.ProductID}
		quantity, ok := remaining[line.ProductID]
		if !allowed[line.InventoryID] || line.ReservationID == uuid.Nil() || line.Quantity <= 0 || !ok ||
			line.Quantity > quantity ||
			seen[key] {
			return false, ErrInvalidItem
		}
		if previous, ok := reservationByInventory[line.InventoryID]; ok && previous != line.ReservationID {
			return false, ErrInvalidItem
		}
		reservationByInventory[line.InventoryID] = line.ReservationID
		remaining[line.ProductID] -= line.Quantity
		seen[key] = true
	}
	if len(shortages) > 0 && !validIngredients(shortages) {
		return false, ErrInvalidItem
	}
	for _, shortage := range shortages {
		quantity, ok := remaining[shortage.ProductID]
		if !ok || shortage.Quantity != quantity {
			return false, ErrInvalidItem
		}
		remaining[shortage.ProductID] = 0
	}
	for _, quantity := range remaining {
		if quantity != 0 {
			return false, ErrInvalidItem
		}
	}
	changed, err := i.ApplyReservation(revision, version, shortages)
	if err != nil || !changed {
		return changed, err
	}
	i.data.Allocations = append([]AllocationLine(nil), allocations...)
	return true, nil
}

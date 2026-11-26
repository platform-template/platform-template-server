package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	driver "github.com/go-sql-driver/mysql"

	"inventory/internal/inventory/domain"
)

type reservationRepository struct {
	ctx    context.Context
	client mysql.ClientContext
}

func NewReservationRepository(ctx context.Context, client mysql.ClientContext) domain.ReservationRepository {
	return &reservationRepository{
		ctx:    ctx,
		client: client,
	}
}

func (r *reservationRepository) GetByID(inventoryID, id uuid.UUID) (*domain.Reservation, error) {
	reservation, err := r.get(id, false)
	if err != nil || reservation == nil {
		return reservation, err
	}
	if reservation.InventoryID() != inventoryID {
		return nil, domain.ErrReservationConflict
	}
	return reservation, nil
}

func (r *reservationRepository) get(id uuid.UUID, lock bool) (*domain.Reservation, error) {
	query := `SELECT inventory_id, status FROM reservations WHERE id = ?`
	if lock {
		query += ` FOR UPDATE`
	}
	var status string
	var inventoryBytes []byte
	err := r.client.QueryRowContext(
		r.ctx,
		"reservation.get",
		query,
		id[:],
	).Scan(&inventoryBytes, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get reservation: %w", err)
	}
	if len(inventoryBytes) != 16 {
		return nil, errors.New("invalid inventory ID")
	}
	var inventoryID uuid.UUID
	copy(inventoryID[:], inventoryBytes)
	rows, err := r.client.QueryContext(
		r.ctx,
		"reservation.items",
		`SELECT product_id, quantity FROM reservation_items WHERE reservation_id = ? ORDER BY product_id`,
		id[:],
	)
	if err != nil {
		return nil, fmt.Errorf("get reservation items: %w", err)
	}
	defer rows.Close()
	items := make([]domain.ReservationItem, 0)
	for rows.Next() {
		var productID []byte
		var quantity int64
		if err := rows.Scan(&productID, &quantity); err != nil {
			return nil, fmt.Errorf("scan reservation item: %w", err)
		}
		if len(productID) != 16 {
			return nil, errors.New("invalid product ID")
		}
		var id uuid.UUID
		copy(id[:], productID)
		items = append(items, domain.ReservationItem{
			ProductID: id,
			Quantity:  quantity,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate reservation items: %w", err)
	}
	reservation, err := domain.RestoreReservation(id, inventoryID, domain.ReservationStatus(status), items)
	if err != nil {
		return nil, fmt.Errorf("restore reservation: %w", err)
	}
	return reservation, nil
}

func (r *reservationRepository) insert(inventoryID, id uuid.UUID, status domain.ReservationStatus) (bool, error) {
	_, err := r.client.ExecContext(
		r.ctx,
		"reservation.insert",
		`INSERT INTO reservations (id, inventory_id, status) VALUES (?, ?, ?)`,
		id[:],
		inventoryID[:],
		string(status),
	)
	var mysqlErr *driver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("insert reservation: %w", err)
	}
	return true, nil
}

func (r *reservationRepository) Reserve(reservation *domain.Reservation) error {
	if reservation == nil || reservation.Status() != domain.ReservationActive {
		return domain.ErrInvalidReservation
	}
	id := reservation.ID()
	created, err := r.insert(reservation.InventoryID(), id, domain.ReservationActive)
	if err != nil {
		return err
	}
	if !created {
		return r.checkExisting(reservation)
	}
	for _, item := range reservation.Items() {
		stock, err := getStock(r.ctx, r.client, reservation.InventoryID(), item.ProductID, true)
		if err != nil {
			return err
		}
		if stock == nil {
			return domain.ErrInsufficientStock
		}
		updated, err := stock.Reserve(item.Quantity)
		if err != nil {
			return err
		}
		_, err = r.client.ExecContext(
			r.ctx,
			"stock.reserve",
			`UPDATE stocks SET reserved_quantity = ? WHERE inventory_id = ? AND product_id = ?`,
			updated.ReservedQuantity,
			inventoryIDBytes(reservation),
			item.ProductID[:],
		)
		if err != nil {
			return fmt.Errorf("reserve stock: %w", err)
		}
		_, err = r.client.ExecContext(
			r.ctx,
			"reservation.item.insert",
			`INSERT INTO reservation_items (reservation_id, product_id, quantity) VALUES (?, ?, ?)`,
			id[:],
			item.ProductID[:],
			item.Quantity,
		)
		if err != nil {
			return fmt.Errorf("insert reservation item: %w", err)
		}
	}
	return nil
}

func (r *reservationRepository) checkExisting(requested *domain.Reservation) error {
	existing, err := r.get(requested.ID(), true)
	if err != nil {
		return err
	}
	if existing == nil {
		return errors.New("reservation disappeared")
	}
	if existing.InventoryID() != requested.InventoryID() {
		return domain.ErrReservationConflict
	}
	if len(existing.Items()) != 0 && !existing.Matches(requested) {
		return domain.ErrReservationConflict
	}
	if existing.Status() == domain.ReservationCancelled {
		return domain.ErrReservationCancelled
	}
	if existing.Status() == domain.ReservationConsumed {
		return domain.ErrReservationConsumed
	}
	return nil
}

func (r *reservationRepository) Release(inventoryID, id uuid.UUID) error {
	if id == uuid.Nil() || inventoryID == uuid.Nil() {
		return domain.ErrInvalidReservation
	}
	created, err := r.insert(inventoryID, id, domain.ReservationCancelled)
	if err != nil {
		return err
	}
	if created {
		return nil
	}
	reservation, err := r.get(id, true)
	if err != nil {
		return err
	}
	if reservation == nil {
		return errors.New("reservation disappeared")
	}
	if reservation.InventoryID() != inventoryID {
		return domain.ErrReservationConflict
	}
	changed, err := reservation.Cancel()
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	for _, item := range reservation.Items() {
		stock, err := getStock(r.ctx, r.client, reservation.InventoryID(), item.ProductID, true)
		if err != nil {
			return err
		}
		if stock == nil {
			return domain.ErrProductNotFound
		}
		updated, err := stock.Release(item.Quantity)
		if err != nil {
			return err
		}
		_, err = r.client.ExecContext(
			r.ctx,
			"stock.release",
			`UPDATE stocks SET reserved_quantity = ? WHERE inventory_id = ? AND product_id = ?`,
			updated.ReservedQuantity,
			inventoryIDBytes(reservation),
			item.ProductID[:],
		)
		if err != nil {
			return fmt.Errorf("release stock: %w", err)
		}
	}
	_, err = r.client.ExecContext(
		r.ctx,
		"reservation.cancel",
		`UPDATE reservations SET status = ? WHERE id = ?`,
		string(domain.ReservationCancelled),
		id[:],
	)
	if err != nil {
		return fmt.Errorf("cancel reservation: %w", err)
	}
	return nil
}

func (r *reservationRepository) Consume(inventoryID, id uuid.UUID) error {
	if id == uuid.Nil() || inventoryID == uuid.Nil() {
		return domain.ErrInvalidReservation
	}
	reservation, err := r.get(id, true)
	if err != nil {
		return err
	}
	if reservation == nil {
		return domain.ErrReservationNotFound
	}
	if reservation.InventoryID() != inventoryID {
		return domain.ErrReservationConflict
	}
	changed, err := reservation.Consume()
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	for _, item := range reservation.Items() {
		stock, err := getStock(r.ctx, r.client, reservation.InventoryID(), item.ProductID, true)
		if err != nil {
			return err
		}
		if stock == nil {
			return domain.ErrProductNotFound
		}
		updated, err := stock.Consume(item.Quantity)
		if err != nil {
			return err
		}
		_, err = r.client.ExecContext(r.ctx, "stock.consume",
			`UPDATE stocks SET quantity = ?, reserved_quantity = ? WHERE inventory_id = ? AND product_id = ?`,
			updated.Quantity, updated.ReservedQuantity, inventoryIDBytes(reservation), item.ProductID[:])
		if err != nil {
			return fmt.Errorf("consume stock: %w", err)
		}
	}
	_, err = r.client.ExecContext(r.ctx, "reservation.consume",
		`UPDATE reservations SET status = ? WHERE id = ?`, string(domain.ReservationConsumed), id[:])
	if err != nil {
		return fmt.Errorf("consume reservation: %w", err)
	}
	return nil
}

func inventoryIDBytes(reservation *domain.Reservation) []byte {
	id := reservation.InventoryID()
	return id[:]
}

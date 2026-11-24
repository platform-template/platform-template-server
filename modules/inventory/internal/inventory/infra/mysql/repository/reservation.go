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

func (r *reservationRepository) GetByID(id uuid.UUID) (*domain.Reservation, error) {
	return r.get(id, false)
}

func (r *reservationRepository) get(id uuid.UUID, lock bool) (*domain.Reservation, error) {
	query := `SELECT status FROM reservations WHERE id = ?`
	if lock {
		query += ` FOR UPDATE`
	}
	var status string
	err := r.client.QueryRowContext(
		r.ctx,
		"reservation.get",
		query,
		id[:],
	).Scan(&status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get reservation: %w", err)
	}
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
	reservation, err := domain.RestoreReservation(id, domain.ReservationStatus(status), items)
	if err != nil {
		return nil, fmt.Errorf("restore reservation: %w", err)
	}
	return reservation, nil
}

func (r *reservationRepository) insert(id uuid.UUID, status domain.ReservationStatus) (bool, error) {
	_, err := r.client.ExecContext(
		r.ctx,
		"reservation.insert",
		`INSERT INTO reservations (id, status) VALUES (?, ?)`,
		id[:],
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
	created, err := r.insert(id, domain.ReservationActive)
	if err != nil {
		return err
	}
	if !created {
		return r.checkExisting(reservation)
	}
	for _, item := range reservation.Items() {
		stock, err := getStock(r.ctx, r.client, item.ProductID, true)
		if err != nil {
			return err
		}
		if stock == nil {
			return domain.ErrProductNotFound
		}
		updated, err := stock.Reserve(item.Quantity)
		if err != nil {
			return err
		}
		_, err = r.client.ExecContext(
			r.ctx,
			"stock.reserve",
			`UPDATE stocks SET reserved_quantity = ? WHERE product_id = ?`,
			updated.ReservedQuantity,
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
	if existing.Status() == domain.ReservationCancelled {
		return domain.ErrReservationCancelled
	}
	if existing.Status() == domain.ReservationConsumed {
		return domain.ErrReservationConsumed
	}
	if !existing.Matches(requested) {
		return domain.ErrReservationConflict
	}
	return nil
}

func (r *reservationRepository) Release(id uuid.UUID) error {
	if id == uuid.Nil() {
		return domain.ErrInvalidReservation
	}
	created, err := r.insert(id, domain.ReservationCancelled)
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
	changed, err := reservation.Cancel()
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	for _, item := range reservation.Items() {
		stock, err := getStock(r.ctx, r.client, item.ProductID, true)
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
			`UPDATE stocks SET reserved_quantity = ? WHERE product_id = ?`,
			updated.ReservedQuantity,
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

func (r *reservationRepository) Consume(id uuid.UUID) error {
	if id == uuid.Nil() {
		return domain.ErrInvalidReservation
	}
	reservation, err := r.get(id, true)
	if err != nil {
		return err
	}
	if reservation == nil {
		return domain.ErrReservationNotFound
	}
	changed, err := reservation.Consume()
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	for _, item := range reservation.Items() {
		stock, err := getStock(r.ctx, r.client, item.ProductID, true)
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
			`UPDATE stocks SET quantity = ?, reserved_quantity = ? WHERE product_id = ?`,
			updated.Quantity, updated.ReservedQuantity, item.ProductID[:])
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

package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"uuid"

	"shopping/internal/shopping/domain"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

type listRepository struct {
	ctx    context.Context
	client mysql.ClientContext
}

func NewListRepository(ctx context.Context, client mysql.ClientContext) domain.ListRepository {
	return &listRepository{ctx: ctx, client: client}
}
func (r *listRepository) Create(plan *domain.List) error {
	id, owner := plan.ID(), plan.OwnerID()
	_, err := r.client.ExecContext(
		r.ctx,
		"shopping.list.create",
		`INSERT INTO shopping_lists (id, owner_id, name) VALUES (?, ?, ?)`,
		id[:],
		owner[:],
		plan.Name(),
	)
	if err != nil {
		return fmt.Errorf("create shopping list: %w", err)
	}
	return nil
}
func (r *listRepository) Get(ownerID, planID uuid.UUID) (*domain.List, error) {
	var name string
	err := r.client.QueryRowContext(r.ctx, "shopping.list.get", `SELECT name FROM shopping_lists WHERE id = ? AND owner_id = ?`, planID[:], ownerID[:]).
		Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get shopping list: %w", err)
	}
	return domain.NewList(planID, ownerID, name)
}
func (r *listRepository) List(ownerID uuid.UUID) ([]*domain.List, error) {
	rows, err := r.client.QueryContext(
		r.ctx,
		"shopping.list.list",
		`SELECT id, name FROM shopping_lists WHERE owner_id = ? ORDER BY id`,
		ownerID[:],
	)
	if err != nil {
		return nil, fmt.Errorf("list shopping lists: %w", err)
	}
	defer rows.Close()
	plans := make([]*domain.List, 0)
	for rows.Next() {
		var raw []byte
		var name string
		if err := rows.Scan(&raw, &name); err != nil {
			return nil, err
		}
		id, err := parseBinaryUUID(raw)
		if err != nil {
			return nil, err
		}
		plan, err := domain.NewList(id, ownerID, name)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, rows.Err()
}
func (r *listRepository) Update(plan *domain.List) error {
	id, owner := plan.ID(), plan.OwnerID()
	result, err := r.client.ExecContext(
		r.ctx,
		"shopping.list.update",
		`UPDATE shopping_lists SET name = ? WHERE id = ? AND owner_id = ?`,
		plan.Name(),
		id[:],
		owner[:],
	)
	if err != nil {
		return fmt.Errorf("update shopping list: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update shopping list result: %w", err)
	}
	if affected == 0 {
		existing, err := r.Get(owner, id)
		if err != nil {
			return err
		}
		if existing == nil {
			return domain.ErrListNotFound
		}
	}
	return nil
}

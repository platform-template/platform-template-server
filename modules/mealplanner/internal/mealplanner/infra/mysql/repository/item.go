package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"uuid"

	"mealplanner/internal/mealplanner/domain"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

type itemRepository struct {
	ctx    context.Context
	client mysql.ClientContext
}

func NewItemRepository(ctx context.Context, client mysql.ClientContext) domain.ItemRepository {
	return &itemRepository{ctx: ctx, client: client}
}
func (r *itemRepository) Create(item *domain.Item) error {
	data := item.Data()
	encoded, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("encode meal item: %w", err)
	}
	_, err = r.client.ExecContext(
		r.ctx,
		"mealitem.create",
		`INSERT INTO meal_items (id, plan_id, scheduled_at, payload, row_version) VALUES (?, ?, ?, ?, 1)`,
		data.ID[:],
		data.PlanID[:],
		data.ScheduledAt,
		encoded,
	)
	if err != nil {
		return fmt.Errorf("create meal item: %w", err)
	}
	return nil
}
func decodeItem(raw []byte, planID, itemID uuid.UUID) (*domain.Item, error) {
	var data domain.ItemData
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, fmt.Errorf("decode meal item: %w", err)
	}
	if data.PlanID != planID || data.ID != itemID {
		return nil, domain.ErrInvalidItem
	}
	return domain.RestoreItem(data)
}
func (r *itemRepository) Get(planID, itemID uuid.UUID) (*domain.Item, uint64, error) {
	var raw []byte
	var version uint64
	err := r.client.QueryRowContext(r.ctx, "mealitem.get", `SELECT payload, row_version FROM meal_items WHERE id = ? AND plan_id = ?`, itemID[:], planID[:]).
		Scan(&raw, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("get meal item: %w", err)
	}
	item, err := decodeItem(raw, planID, itemID)
	return item, version, err
}
func (r *itemRepository) List(planID uuid.UUID, from, to time.Time) ([]*domain.Item, error) {
	if from.IsZero() || to.IsZero() || !from.Before(to) {
		return nil, domain.ErrInvalidItem
	}
	rows, err := r.client.QueryContext(
		r.ctx,
		"mealitem.list",
		`SELECT id, payload FROM meal_items WHERE plan_id = ? AND scheduled_at >= ? AND scheduled_at < ? ORDER BY scheduled_at, id`,
		planID[:],
		from.UTC(),
		to.UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf("list meal items: %w", err)
	}
	defer rows.Close()
	items := make([]*domain.Item, 0)
	for rows.Next() {
		var idBytes, raw []byte
		if err := rows.Scan(&idBytes, &raw); err != nil {
			return nil, err
		}
		id, err := parseBinaryUUID(idBytes)
		if err != nil {
			return nil, err
		}
		item, err := decodeItem(raw, planID, id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}
func (r *itemRepository) Update(item *domain.Item, version uint64) error {
	if version == 0 || version == ^uint64(0) {
		return domain.ErrConcurrentUpdate
	}
	data := item.Data()
	encoded, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("encode meal item: %w", err)
	}
	result, err := r.client.ExecContext(
		r.ctx,
		"mealitem.update",
		`UPDATE meal_items SET scheduled_at = ?, payload = ?, row_version = row_version + 1 WHERE id = ? AND plan_id = ? AND row_version = ?`,
		data.ScheduledAt,
		encoded,
		data.ID[:],
		data.PlanID[:],
		version,
	)
	if err != nil {
		return fmt.Errorf("update meal item: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update meal item result: %w", err)
	}
	if affected != 1 {
		return domain.ErrConcurrentUpdate
	}
	return nil
}

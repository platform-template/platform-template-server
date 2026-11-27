package repository

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"uuid"

	"shopping/internal/shopping/domain"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	driver "github.com/go-sql-driver/mysql"
)

type demandRepository struct {
	ctx    context.Context
	client mysql.ClientContext
}

func NewDemandRepository(ctx context.Context, client mysql.ClientContext) domain.DemandRepository {
	return &demandRepository{ctx: ctx, client: client}
}

func (r *demandRepository) CreateManual(demand *domain.Demand) error {
	data := demand.Data()
	if data.Source != nil {
		return domain.ErrInvalidDemand
	}
	_, err := r.client.ExecContext(
		r.ctx,
		"demand.create_manual",
		`INSERT INTO shopping_demands (id,list_id,product_id,required,purchased,cancelled) VALUES (?, ?, ?, ?, 0, FALSE)`,
		data.ID[:],
		data.ListID[:],
		data.ProductID[:],
		data.Required,
	)
	if err != nil {
		return fmt.Errorf("create manual demand: %w", err)
	}
	return nil
}

func (r *demandRepository) Get(listID, demandID uuid.UUID) (*domain.Demand, error) {
	return r.get(listID, demandID, "")
}

func (r *demandRepository) List(listID uuid.UUID) ([]*domain.Demand, error) {
	rows, err := r.client.QueryContext(
		r.ctx,
		"demand.list",
		`SELECT id,product_id,required,purchased,cancelled,meal_plan_id,meal_item_id,revision FROM shopping_demands WHERE list_id = ? ORDER BY id`,
		listID[:],
	)
	if err != nil {
		return nil, fmt.Errorf("list shopping demands: %w", err)
	}
	defer rows.Close()
	result := make([]*domain.Demand, 0)
	for rows.Next() {
		var raw, productID, planID, itemID, revision []byte
		var required, purchased int64
		var cancelled bool
		if err := rows.Scan(
			&raw,
			&productID,
			&required,
			&purchased,
			&cancelled,
			&planID,
			&itemID,
			&revision,
		); err != nil {
			return nil, err
		}
		id, err := parseBinaryUUID(raw)
		if err != nil {
			return nil, err
		}
		product, err := parseBinaryUUID(productID)
		if err != nil {
			return nil, err
		}
		data := domain.DemandData{
			ID:        id,
			ListID:    listID,
			ProductID: product,
			Required:  required,
			Purchased: purchased,
			Cancelled: cancelled,
		}
		if planID != nil || itemID != nil || revision != nil {
			plan, err := parseBinaryUUID(planID)
			if err != nil {
				return nil, err
			}
			item, err := parseBinaryUUID(itemID)
			if err != nil {
				return nil, err
			}
			rev, err := parseBinaryUUID(revision)
			if err != nil {
				return nil, err
			}
			data.Source = &domain.Source{MealPlanID: plan, MealItemID: item, Revision: rev}
		}
		demand, err := domain.RestoreDemand(data)
		if err != nil {
			return nil, err
		}
		result = append(result, demand)
	}
	return result, rows.Err()
}

func (r *demandRepository) Lock(listID, demandID uuid.UUID) (*domain.Demand, error) {
	return r.get(listID, demandID, " FOR UPDATE")
}

func (r *demandRepository) get(listID, demandID uuid.UUID, lock string) (*domain.Demand, error) {
	var productID []byte
	var required, purchased int64
	var cancelled bool
	var planID, itemID, revision []byte
	err := r.client.QueryRowContext(
		r.ctx,
		"demand.get",
		`SELECT product_id,required,purchased,cancelled,meal_plan_id,meal_item_id,revision FROM shopping_demands WHERE id = ? AND list_id = ?`+lock,
		demandID[:],
		listID[:],
	).Scan(&productID, &required, &purchased, &cancelled, &planID, &itemID, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get shopping demand: %w", err)
	}
	product, err := parseBinaryUUID(productID)
	if err != nil {
		return nil, err
	}
	data := domain.DemandData{
		ID:        demandID,
		ListID:    listID,
		ProductID: product,
		Required:  required,
		Purchased: purchased,
		Cancelled: cancelled,
	}
	if planID != nil || itemID != nil || revision != nil {
		plan, err := parseBinaryUUID(planID)
		if err != nil {
			return nil, err
		}
		item, err := parseBinaryUUID(itemID)
		if err != nil {
			return nil, err
		}
		rev, err := parseBinaryUUID(revision)
		if err != nil {
			return nil, err
		}
		data.Source = &domain.Source{MealPlanID: plan, MealItemID: item, Revision: rev}
	}
	return domain.RestoreDemand(data)
}

func (r *demandRepository) Update(demand *domain.Demand) error {
	data := demand.Data()
	_, err := r.client.ExecContext(
		r.ctx,
		"demand.update",
		`UPDATE shopping_demands SET required = ?, purchased = ?, cancelled = ? WHERE id = ? AND list_id = ?`,
		data.Required,
		data.Purchased,
		data.Cancelled,
		data.ID[:],
		data.ListID[:],
	)
	if err != nil {
		return fmt.Errorf("update shopping demand: %w", err)
	}
	return nil
}
func (r *demandRepository) LockSource(listID uuid.UUID, source domain.Source) (bool, error) {
	_, err := r.client.ExecContext(
		r.ctx,
		"demand.ensure_source",
		`INSERT INTO shopping_source_states (revision,list_id,meal_plan_id,meal_item_id,cancelled) VALUES (?, ?, ?, ?, FALSE) ON DUPLICATE KEY UPDATE revision = revision`,
		source.Revision[:],
		listID[:],
		source.MealPlanID[:],
		source.MealItemID[:],
	)
	if err != nil {
		return false, fmt.Errorf("ensure demand source: %w", err)
	}
	var storedList, storedPlan, storedItem []byte
	var cancelled bool
	err = r.client.QueryRowContext(r.ctx, "demand.lock_source", `SELECT list_id,meal_plan_id,meal_item_id,cancelled FROM shopping_source_states WHERE revision = ? FOR UPDATE`, source.Revision[:]).
		Scan(&storedList, &storedPlan, &storedItem, &cancelled)
	if err != nil {
		return false, fmt.Errorf("lock demand source: %w", err)
	}
	if !bytes.Equal(storedList, listID[:]) || !bytes.Equal(storedPlan, source.MealPlanID[:]) ||
		!bytes.Equal(storedItem, source.MealItemID[:]) {
		return false, domain.ErrDemandConflict
	}
	return cancelled, nil
}
func (r *demandRepository) CancelSource(listID uuid.UUID, source domain.Source) (bool, error) {
	cancelled, err := r.LockSource(listID, source)
	if err != nil || cancelled {
		return false, err
	}
	_, err = r.client.ExecContext(
		r.ctx,
		"demand.cancel_source",
		`UPDATE shopping_demands SET cancelled = TRUE WHERE list_id = ? AND meal_plan_id = ? AND meal_item_id = ? AND revision = ? AND cancelled = FALSE`,
		listID[:],
		source.MealPlanID[:],
		source.MealItemID[:],
		source.Revision[:],
	)
	if err != nil {
		return false, fmt.Errorf("cancel source demands: %w", err)
	}
	_, err = r.client.ExecContext(
		r.ctx,
		"demand.cancel_marker",
		`UPDATE shopping_source_states SET cancelled = TRUE WHERE revision = ?`,
		source.Revision[:],
	)
	if err != nil {
		return false, fmt.Errorf("mark demand source cancelled: %w", err)
	}
	return true, nil
}

func (r *demandRepository) RegisterAllocation(revision uuid.UUID, hash [32]byte) (bool, error) {
	_, err := r.client.ExecContext(
		r.ctx,
		"demand.register_allocation",
		`INSERT INTO shopping_allocation_results (revision,request_hash) VALUES (?, ?)`,
		revision[:],
		hash[:],
	)
	var mysqlErr *driver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		var saved []byte
		if err := r.client.QueryRowContext(r.ctx, "demand.get_allocation", `SELECT request_hash FROM shopping_allocation_results WHERE revision = ? FOR UPDATE`, revision[:]).
			Scan(&saved); err != nil {
			return false, fmt.Errorf("get allocation result: %w", err)
		}
		if !bytes.Equal(saved, hash[:]) {
			return false, domain.ErrDemandConflict
		}
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("register allocation result: %w", err)
	}
	return true, nil
}

func (r *demandRepository) CreateAutomatic(demand *domain.Demand) (bool, error) {
	data := demand.Data()
	if data.Source == nil {
		return false, domain.ErrInvalidDemand
	}
	source := *data.Source
	_, err := r.client.ExecContext(
		r.ctx,
		"demand.create",
		`INSERT INTO shopping_demands (id,list_id,product_id,meal_plan_id,meal_item_id,revision,required,purchased,cancelled) VALUES (?, ?, ?, ?, ?, ?, ?, 0, FALSE)`,
		data.ID[:],
		data.ListID[:],
		data.ProductID[:],
		source.MealPlanID[:],
		source.MealItemID[:],
		source.Revision[:],
		data.Required,
	)
	var mysqlErr *driver.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1062 {
		var quantity int64
		queryErr := r.client.QueryRowContext(r.ctx, "demand.existing", `SELECT required FROM shopping_demands WHERE list_id = ? AND meal_plan_id = ? AND meal_item_id = ? AND revision = ? AND product_id = ? FOR UPDATE`, data.ListID[:], source.MealPlanID[:], source.MealItemID[:], source.Revision[:], data.ProductID[:]).
			Scan(&quantity)
		if queryErr != nil {
			return false, fmt.Errorf("get duplicate demand: %w", queryErr)
		}
		if quantity != data.Required {
			return false, domain.ErrDemandConflict
		}
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("create automatic demand: %w", err)
	}
	return true, nil
}
func (r *demandRepository) ListBySource(listID uuid.UUID, source domain.Source) ([]*domain.Demand, error) {
	rows, err := r.client.QueryContext(
		r.ctx,
		"demand.list_source",
		`SELECT id, product_id, required, purchased, cancelled FROM shopping_demands WHERE list_id = ? AND meal_plan_id = ? AND meal_item_id = ? AND revision = ? ORDER BY product_id`,
		listID[:],
		source.MealPlanID[:],
		source.MealItemID[:],
		source.Revision[:],
	)
	if err != nil {
		return nil, fmt.Errorf("list demands: %w", err)
	}
	defer rows.Close()
	result := make([]*domain.Demand, 0)
	for rows.Next() {
		var idBytes, productBytes []byte
		var required, purchased int64
		var cancelled bool
		if err := rows.Scan(&idBytes, &productBytes, &required, &purchased, &cancelled); err != nil {
			return nil, err
		}
		id, err := parseBinaryUUID(idBytes)
		if err != nil {
			return nil, err
		}
		product, err := parseBinaryUUID(productBytes)
		if err != nil {
			return nil, err
		}
		demand, err := domain.RestoreDemand(
			domain.DemandData{
				ID:        id,
				ListID:    listID,
				ProductID: product,
				Required:  required,
				Purchased: purchased,
				Cancelled: cancelled,
				Source:    &source,
			},
		)
		if err != nil {
			return nil, err
		}
		result = append(result, demand)
	}
	return result, rows.Err()
}

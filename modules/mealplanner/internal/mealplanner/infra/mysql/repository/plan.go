package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"uuid"

	"mealplanner/internal/mealplanner/domain"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

type planRepository struct {
	ctx    context.Context
	client mysql.ClientContext
}

func NewPlanRepository(ctx context.Context, client mysql.ClientContext) domain.PlanRepository {
	return &planRepository{ctx: ctx, client: client}
}
func (r *planRepository) Create(plan *domain.Plan) error {
	id, owner := plan.ID(), plan.OwnerID()
	_, err := r.client.ExecContext(
		r.ctx,
		"mealplan.create",
		`INSERT INTO meal_plans (id, owner_id, name) VALUES (?, ?, ?)`,
		id[:],
		owner[:],
		plan.Name(),
	)
	if err != nil {
		return fmt.Errorf("create meal plan: %w", err)
	}
	return nil
}
func (r *planRepository) Get(ownerID, planID uuid.UUID) (*domain.Plan, error) {
	var name string
	err := r.client.QueryRowContext(r.ctx, "mealplan.get", `SELECT name FROM meal_plans WHERE id = ? AND owner_id = ?`, planID[:], ownerID[:]).
		Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get meal plan: %w", err)
	}
	return domain.NewPlan(planID, ownerID, name)
}
func (r *planRepository) List(ownerID uuid.UUID) ([]*domain.Plan, error) {
	rows, err := r.client.QueryContext(
		r.ctx,
		"mealplan.list",
		`SELECT id, name FROM meal_plans WHERE owner_id = ? ORDER BY id`,
		ownerID[:],
	)
	if err != nil {
		return nil, fmt.Errorf("list meal plans: %w", err)
	}
	defer rows.Close()
	plans := make([]*domain.Plan, 0)
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
		plan, err := domain.NewPlan(id, ownerID, name)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, rows.Err()
}
func (r *planRepository) Update(plan *domain.Plan) error {
	id, owner := plan.ID(), plan.OwnerID()
	result, err := r.client.ExecContext(
		r.ctx,
		"mealplan.update",
		`UPDATE meal_plans SET name = ? WHERE id = ? AND owner_id = ?`,
		plan.Name(),
		id[:],
		owner[:],
	)
	if err != nil {
		return fmt.Errorf("update meal plan: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update meal plan result: %w", err)
	}
	if affected == 0 {
		existing, err := r.Get(owner, id)
		if err != nil {
			return err
		}
		if existing == nil {
			return domain.ErrPlanNotFound
		}
	}
	return nil
}

package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"

	"authz/internal/authz/domain"
)

type initialSuperuserRepository struct {
	ctx    context.Context
	client mysql.ClientContext
}

func NewInitialSuperuserRepository(
	ctx context.Context,
	client mysql.ClientContext,
) domain.InitialSuperuserRepository {
	return &initialSuperuserRepository{ctx: ctx, client: client}
}

func (r *initialSuperuserRepository) Bootstrap(
	assignment *domain.SystemRoleAssignment,
) error {
	var lockID int
	if err := r.client.QueryRowContext(
		r.ctx,
		"authz.initial_superuser.lock",
		`SELECT id FROM initial_superuser_lock WHERE id = 1 FOR UPDATE`,
	).Scan(&lockID); err != nil {
		return fmt.Errorf("lock initial superuser: %w", err)
	}
	data := assignment.Data()
	var existingUserID string
	err := r.client.QueryRowContext(
		r.ctx,
		"authz.initial_superuser.find",
		`SELECT user_id FROM system_roles WHERE role = 'superuser' LIMIT 1`,
	).Scan(&existingUserID)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = r.client.ExecContext(
			r.ctx,
			"authz.initial_superuser.store",
			`INSERT INTO system_roles (user_id, role) VALUES (?, ?)`,
			data.UserID.String(),
			string(data.Role),
		)
		if err != nil {
			return fmt.Errorf("store initial superuser: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("find initial superuser: %w", err)
	}
	if existingUserID == data.UserID.String() {
		return nil
	}
	return domain.ErrInitialSuperuserExists
}

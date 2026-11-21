package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"

	"authz/internal/authz/domain"
)

type systemRoleRepository struct {
	ctx    context.Context
	client mysql.ClientContext
}

func NewSystemRoleRepository(
	ctx context.Context,
	client mysql.ClientContext,
) domain.SystemRoleRepository {
	return &systemRoleRepository{ctx: ctx, client: client}
}

func (r *systemRoleRepository) Store(
	assignment *domain.SystemRoleAssignment,
) error {
	data := assignment.Data()
	_, err := r.client.ExecContext(r.ctx, "authz.system_role.store", `
INSERT INTO system_roles (user_id, role)
VALUES (?, ?)
ON DUPLICATE KEY UPDATE role = ?
`, data.UserID.String(), string(data.Role), string(data.Role))
	if err != nil {
		return fmt.Errorf("store system role: %w", err)
	}
	return nil
}

func (r *systemRoleRepository) FindByUserID(
	userID uuid.UUID,
) (*domain.SystemRoleAssignment, error) {
	var storedUserID, storedRole string
	err := r.client.QueryRowContext(r.ctx, "authz.system_role.find", `
SELECT user_id, role
FROM system_roles
WHERE user_id = ?
`, userID.String()).Scan(&storedUserID, &storedRole)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find system role: %w", err)
	}
	parsedUserID, err := uuid.Parse(storedUserID)
	if err != nil {
		return nil, fmt.Errorf("parse system role user ID: %w", err)
	}
	role, err := domain.NewSystemRole(storedRole)
	if err != nil {
		return nil, fmt.Errorf("restore system role: %w", err)
	}
	assignment, err := domain.NewSystemRoleAssignment(parsedUserID, role)
	if err != nil {
		return nil, fmt.Errorf("restore system role assignment: %w", err)
	}
	return assignment, nil
}

func (r *systemRoleRepository) Delete(userID uuid.UUID) error {
	_, err := r.client.ExecContext(r.ctx, "authz.system_role.delete", `
DELETE FROM system_roles
WHERE user_id = ?
`, userID.String())
	if err != nil {
		return fmt.Errorf("delete system role: %w", err)
	}
	return nil
}

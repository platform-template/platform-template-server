package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	mysqldriver "github.com/go-sql-driver/mysql"

	"authz/internal/authz/domain"
)

type moduleGrantRepository struct {
	ctx    context.Context
	client mysql.ClientContext
}

func NewModuleGrantRepository(
	ctx context.Context,
	client mysql.ClientContext,
) domain.ModuleGrantRepository {
	return &moduleGrantRepository{ctx: ctx, client: client}
}

func (r *moduleGrantRepository) Store(grant *domain.ModuleGrant) error {
	data := grant.Data()
	_, err := r.client.ExecContext(r.ctx, "authz.module_grant.store", `
INSERT INTO module_grants (user_id, module_key, granted_by, granted_at)
VALUES (?, ?, ?, ?)
`, data.UserID.String(), data.ModuleKey.String(), nullableUUID(data.GrantedBy), data.GrantedAt)
	if err == nil {
		return nil
	}
	if isDuplicateKey(err) {
		return fmt.Errorf(
			"%w: %v",
			domain.ErrModuleGrantAlreadyExists,
			err,
		)
	}
	return fmt.Errorf("store module grant: %w", err)
}

func (r *moduleGrantRepository) FindByUserIDAndModuleKey(
	userID uuid.UUID,
	moduleKey domain.ModuleKey,
) (*domain.ModuleGrant, error) {
	return r.findOne("authz.module_grant.find", `
SELECT user_id, module_key, granted_by, granted_at
FROM module_grants
WHERE user_id = ? AND module_key = ?
`, userID.String(), moduleKey.String())
}

func (r *moduleGrantRepository) FindAllByUserID(
	userID uuid.UUID,
) ([]*domain.ModuleGrant, error) {
	rows, err := r.client.QueryContext(
		r.ctx,
		"authz.module_grant.find_all",
		`
SELECT user_id, module_key, granted_by, granted_at
FROM module_grants
WHERE user_id = ?
ORDER BY module_key
`,
		userID.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("find module grants: %w", err)
	}
	defer rows.Close()

	grants := make([]*domain.ModuleGrant, 0)
	for rows.Next() {
		grant, err := scanModuleGrant(rows)
		if err != nil {
			return nil, err
		}
		grants = append(grants, grant)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate module grants: %w", err)
	}
	return grants, nil
}

func (r *moduleGrantRepository) Delete(
	userID uuid.UUID,
	moduleKey domain.ModuleKey,
) error {
	_, err := r.client.ExecContext(r.ctx, "authz.module_grant.delete", `
DELETE FROM module_grants
WHERE user_id = ? AND module_key = ?
`, userID.String(), moduleKey.String())
	if err != nil {
		return fmt.Errorf("delete module grant: %w", err)
	}
	return nil
}

func (r *moduleGrantRepository) findOne(
	queryID, query string,
	args ...any,
) (*domain.ModuleGrant, error) {
	grant, err := scanModuleGrant(
		r.client.QueryRowContext(r.ctx, queryID, query, args...),
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return grant, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanModuleGrant(row rowScanner) (*domain.ModuleGrant, error) {
	var userID, moduleKey string
	var grantedBy sql.NullString
	var grantedAt time.Time
	if err := row.Scan(
		&userID,
		&moduleKey,
		&grantedBy,
		&grantedAt,
	); err != nil {
		return nil, err
	}
	parsedUserID, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("parse module grant user ID: %w", err)
	}
	parsedModuleKey, err := domain.NewModuleKey(moduleKey)
	if err != nil {
		return nil, fmt.Errorf("restore module grant key: %w", err)
	}
	var parsedGrantedBy *uuid.UUID
	if grantedBy.Valid {
		issuer, err := uuid.Parse(grantedBy.String)
		if err != nil {
			return nil, fmt.Errorf("parse module grant issuer: %w", err)
		}
		parsedGrantedBy = &issuer
	}
	grant, err := domain.NewModuleGrant(
		parsedUserID,
		parsedModuleKey,
		parsedGrantedBy,
		grantedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("restore module grant: %w", err)
	}
	return grant, nil
}

func nullableUUID(value *uuid.UUID) any {
	if value == nil {
		return nil
	}
	return value.String()
}

func isDuplicateKey(err error) bool {
	var mysqlErr *mysqldriver.MySQLError
	return errors.As(err, &mysqlErr) && mysqlErr.Number == 1062
}

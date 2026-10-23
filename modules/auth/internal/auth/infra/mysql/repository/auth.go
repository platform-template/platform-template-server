package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"

	"auth/internal/auth/domain"
)

type authRepository struct {
	ctx    context.Context
	client mysql.ClientContext
}

func NewAuthRepository(ctx context.Context, client mysql.ClientContext) domain.AuthRepository {
	return &authRepository{
		ctx:    ctx,
		client: client,
	}
}

func (r *authRepository) Store(credential *domain.PasswordCredential) error {
	userID := credential.UserID()

	_, err := r.client.ExecContext(r.ctx, "auth.store", `
INSERT INTO password_credentials (user_id, login, password_hash)
VALUES (?, ?, ?)
`, userID.String(), credential.Login(), credential.PasswordHash())

	if err != nil {
		return fmt.Errorf("store password credential: %w", err)
	}

	return nil
}

func (r *authRepository) FindByUserID(userID uuid.UUID) (*domain.PasswordCredential, error) {
	var login, passwordHash string

	err := r.client.QueryRowContext(r.ctx, "auth.find_by_user_id", `
SELECT login, password_hash
FROM password_credentials
WHERE user_id = ?
`, userID.String()).Scan(&login, &passwordHash)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find password credential: %w", err)
	}

	credential, err := domain.NewPasswordCredential(userID, login, passwordHash)
	if err != nil {
		return nil, fmt.Errorf("restore password credential: %w", err)
	}

	return credential, nil
}

func (r *authRepository) FindByLogin(login string) (*domain.PasswordCredential, error) {
	var userID, passwordHash string

	err := r.client.QueryRowContext(r.ctx, "auth.find_by_login", `
SELECT user_id, password_hash
FROM password_credentials
WHERE login = ?
`, login).Scan(&userID, &passwordHash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find password credential by login: %w", err)
	}

	parsedUserID, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("parse credential user ID: %w", err)
	}
	credential, err := domain.NewPasswordCredential(parsedUserID, login, passwordHash)
	if err != nil {
		return nil, fmt.Errorf("restore password credential: %w", err)
	}
	return credential, nil
}

func (r *authRepository) Change(credential *domain.PasswordCredential) error {
	userID := credential.UserID()

	_, err := r.client.ExecContext(r.ctx, "auth.change", `
UPDATE password_credentials
SET password_hash = ?
WHERE user_id = ?;
`, credential.PasswordHash(), userID.String())

	if err != nil {
		return fmt.Errorf("update password credential: %w", err)
	}

	return nil
}

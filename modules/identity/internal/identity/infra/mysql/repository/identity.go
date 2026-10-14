package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"

	"identity/internal/identity/domain"
)

func NewIdentityRepository(ctx context.Context, client mysql.ClientContext) domain.IdentityRepository {
	return &identityRepository{
		ctx:    ctx,
		client: client,
	}
}

type identityRepository struct {
	ctx    context.Context
	client mysql.ClientContext
}

func (r *identityRepository) NextID() (uuid.UUID, error) {
	return uuid.New(), nil
}

func (r *identityRepository) Store(user *domain.User) error {
	userData := user.Data()
	_, err := r.client.ExecContext(r.ctx, "identity.store", `
INSERT INTO users (id, first_name, last_name, middle_name, login, email)
VALUES (?, ?, ?, ?, ?, ?)
`,
		userData.ID.String(),
		userData.FirstName,
		userData.LastName,
		userData.MiddleName,
		userData.Login,
		userData.Email.String())

	if err != nil {
		return fmt.Errorf("store user: %w", err)
	}

	return nil
}

func (r *identityRepository) FindByLogin(login string) (*domain.User, error) {
	var id, firstName, lastName, middleName, storedLogin, email string

	err := r.client.QueryRowContext(r.ctx, "identity.find_by_login", `
SELECT id, first_name, last_name, middle_name, login, email
FROM users
WHERE login = ?
`, login).Scan(&id, &firstName, &lastName, &middleName, &storedLogin, &email)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find user by login: %w", err)
	}

	userID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("parse user ID: %w", err)
	}

	user, err := domain.NewUser(userID, firstName, lastName, middleName, storedLogin, domain.Email(email))
	if err != nil {
		return nil, fmt.Errorf("restore user: %w", err)
	}

	return user, nil
}

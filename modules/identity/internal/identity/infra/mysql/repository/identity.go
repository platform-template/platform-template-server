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
INSERT INTO users (id, first_name, last_name, middle_name, login_snapshot, email)
VALUES (?, ?, ?, ?, ?, ?)
`,
		userData.ID.String(),
		userData.FirstName,
		userData.LastName,
		userData.MiddleName,
		userData.LoginSnapshot,
		userData.Email.String())

	if err != nil {
		return fmt.Errorf("store user: %w", err)
	}

	return nil
}

func (r *identityRepository) FindByLoginSnapshot(loginSnapshot string) (*domain.User, error) {
	var id, firstName, lastName, storedLoginSnapshot, email string
	var middleName sql.NullString

	err := r.client.QueryRowContext(r.ctx, "identity.find_by_login_snapshot", `
SELECT id, first_name, last_name, middle_name, login_snapshot, email
FROM users
WHERE login_snapshot = ?
`, loginSnapshot).Scan(&id, &firstName, &lastName, &middleName, &storedLoginSnapshot, &email)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find user by login snapshot: %w", err)
	}

	userID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("parse user ID: %w", err)
	}

	user, err := domain.NewUser(userID, firstName, lastName, nullableString(middleName), storedLoginSnapshot, domain.Email(email))
	if err != nil {
		return nil, fmt.Errorf("restore user: %w", err)
	}

	return user, nil
}

func (r *identityRepository) FindByID(userID uuid.UUID) (*domain.User, error) {
	return r.findOne("identity.find_by_id", `
SELECT id, first_name, last_name, middle_name, login_snapshot, email
FROM users
WHERE id = ?
`, userID.String())
}

func (r *identityRepository) Update(user *domain.User) error {
	data := user.Data()
	_, err := r.client.ExecContext(r.ctx, "identity.update", `
UPDATE users
SET first_name = ?, last_name = ?, middle_name = ?, email = ?
WHERE id = ?
`, data.FirstName, data.LastName, data.MiddleName, data.Email.String(), data.ID.String())
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}

	return nil
}

func (r *identityRepository) findOne(queryName, query, argument string) (*domain.User, error) {
	var id, firstName, lastName, storedLoginSnapshot, email string
	var middleName sql.NullString

	err := r.client.QueryRowContext(
		r.ctx,
		queryName,
		query,
		argument,
	).Scan(
		&id,
		&firstName,
		&lastName,
		&middleName,
		&storedLoginSnapshot,
		&email,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find user: %w", err)
	}

	parsedID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("parse user ID: %w", err)
	}
	user, err := domain.NewUser(
		parsedID,
		firstName,
		lastName,
		nullableString(middleName),
		storedLoginSnapshot,
		domain.Email(email),
	)
	if err != nil {
		return nil, fmt.Errorf("restore user: %w", err)
	}

	return user, nil
}

func nullableString(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

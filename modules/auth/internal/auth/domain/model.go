package domain

import (
	"errors"
	"uuid"
)

var (
	ErrPasswordUnchanged       = errors.New("new password must differ from current password")
	ErrCredentialNotFound      = errors.New("password credential not found")
	ErrCredentialAlreadyExists = errors.New("password credential already exists")
	ErrInvalidPassword         = errors.New("invalid password")
	ErrInvalidUserID           = errors.New("invalid user ID")
	ErrInvalidPasswordHash     = errors.New("invalid password hash")
)

type PasswordCredential struct { // TODO: перенсти сюда login
	userID       uuid.UUID
	passwordHash string
}

func NewPasswordCredential(userID uuid.UUID, passwordHash string) (*PasswordCredential, error) {
	if userID == uuid.Nil() {
		return nil, ErrInvalidUserID
	}
	if passwordHash == "" {
		return nil, ErrInvalidPasswordHash
	}

	return &PasswordCredential{
		userID:       userID,
		passwordHash: passwordHash,
	}, nil
}

func (pc *PasswordCredential) UserID() uuid.UUID {
	return pc.userID
}

func (pc *PasswordCredential) PasswordHash() string {
	return pc.passwordHash
}

func (pc *PasswordCredential) changePasswordHash(passwordHash string) error {
	if passwordHash == "" {
		return ErrInvalidPasswordHash
	}

	pc.passwordHash = passwordHash
	return nil
}

type AuthRepository interface {
	Store(*PasswordCredential) error
	Change(*PasswordCredential) error
	FindByUserID(uuid.UUID) (*PasswordCredential, error)
}

package domain

import (
	"errors"
	"strings"
	"uuid"
)

var (
	ErrPasswordUnchanged       = errors.New("new password must differ from current password")
	ErrCredentialNotFound      = errors.New("password credential not found")
	ErrCredentialAlreadyExists = errors.New("password credential already exists")
	ErrInvalidPassword         = errors.New("invalid password")
	ErrInvalidUserID           = errors.New("invalid user ID")
	ErrInvalidLogin            = errors.New("invalid login")
	ErrInvalidPasswordHash     = errors.New("invalid password hash")
)

type PasswordCredential struct {
	userID       uuid.UUID
	login        string
	passwordHash string
}

func NewPasswordCredential(userID uuid.UUID, login, passwordHash string) (*PasswordCredential, error) {
	if userID == uuid.Nil() {
		return nil, ErrInvalidUserID
	}
	login = strings.TrimSpace(login)
	if login == "" {
		return nil, ErrInvalidLogin
	}
	if passwordHash == "" {
		return nil, ErrInvalidPasswordHash
	}

	return &PasswordCredential{
		userID:       userID,
		login:        login,
		passwordHash: passwordHash,
	}, nil
}

func (pc *PasswordCredential) Login() string {
	return pc.login
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
	FindByLogin(string) (*PasswordCredential, error)
}

package domain

import (
	"errors"
	"net/mail"
	"strings"
	"uuid"
)

var (
	ErrInvalidUserID    = errors.New("invalid user ID")
	ErrInvalidEmail     = errors.New("invalid email")
	ErrInvalidFirstName = errors.New("invalid first name")
	ErrInvalidLastName  = errors.New("invalid last name")
	ErrInvalidLogin     = errors.New("invalid login")
)

type Email string

func NewEmail(raw string) (Email, error) {
	value := strings.TrimSpace(raw)

	addr, err := mail.ParseAddress(value)
	if err != nil || addr.Address != value {
		return "", ErrInvalidEmail
	}

	return Email(value), nil
}

func (e Email) String() string {
	return string(e)
}

type User struct {
	id         uuid.UUID
	firstName  string
	lastName   string
	middleName string
	login      string
	email      Email
}

func NewUser(
	id uuid.UUID,
	firstName string,
	lastName string,
	middleName string,
	login string,
	email Email,
) (*User, error) {
	if id == uuid.Nil() {
		return nil, ErrInvalidUserID
	}
	if firstName == "" {
		return nil, ErrInvalidFirstName
	}
	if lastName == "" {
		return nil, ErrInvalidLastName
	}
	if login == "" {
		return nil, ErrInvalidLogin
	}
	if email == "" {
		return nil, ErrInvalidEmail
	}

	return &User{
		id:         id,
		firstName:  firstName,
		lastName:   lastName,
		middleName: middleName,
		login:      login,
		email:      email,
	}, nil
}

type IdentityRepository interface {
	NextID() (uuid.UUID, error)
	Store(*User) error
	FindByLogin(string) (*User, error)
}

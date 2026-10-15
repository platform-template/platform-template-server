package app

import "errors"

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrLoginAlreadyExists = errors.New("login already exists")
	ErrInvalidSession     = errors.New("invalid session")
	ErrInvalidUserData    = errors.New("invalid user data")
)

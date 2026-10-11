package domain

import "unicode/utf8"

func ValidatePassword(password string) error {
	length := utf8.RuneCountInString(password)

	if length < 8 {
		return ErrInvalidPassword
	}

	return nil
}

type PasswordHasher interface {
	Hash(string) (string, error)
	Verify(password, hash string) (bool, error)
}

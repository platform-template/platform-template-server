package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const (
	validUnicodePassword = "пароль12" //nolint:gosec
	shortUnicodePassword = "привет"   //nolint:gosec
)

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  error
	}{
		{
			name:     "valid password",
			password: "secure-password-123",
		},
		{
			name:     "minimum length",
			password: "12345678",
		},
		{
			name:     "unicode password",
			password: validUnicodePassword,
		},
		{
			name:     "too short",
			password: "1234567",
			wantErr:  ErrInvalidPassword,
		},
		{
			name:     "empty password",
			password: "",
			wantErr:  ErrInvalidPassword,
		},
		{
			name:     "short unicode password",
			password: shortUnicodePassword,
			wantErr:  ErrInvalidPassword,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.password)

			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

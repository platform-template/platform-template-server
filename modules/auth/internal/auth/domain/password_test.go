package domain

import (
	"errors"
	"testing"
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
			password: "пароль12",
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
			password: "привет",
			wantErr:  ErrInvalidPassword,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePassword(tt.password)

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}

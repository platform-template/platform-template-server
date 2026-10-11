package domain

import (
	"errors"
	"testing"

	"uuid"
)

func TestNewPasswordCredential(t *testing.T) {
	validID := uuid.New()

	tests := []struct {
		name    string
		userID  uuid.UUID
		hash    string
		wantErr error
	}{
		{
			name:   "valid credential",
			userID: validID,
			hash:   "password-hash",
		},
		{
			name:    "invalid user ID",
			userID:  uuid.Nil(),
			hash:    "password-hash",
			wantErr: ErrInvalidUserID,
		},
		{
			name:    "empty password hash",
			userID:  validID,
			hash:    "",
			wantErr: ErrInvalidPasswordHash,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credential, err := NewPasswordCredential(tt.userID, tt.hash)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}

			if tt.wantErr != nil {
				if credential != nil {
					t.Error("expected nil credential on error")
				}
				return
			}

			if credential == nil {
				t.Fatal("expected non-nil credential")
			}

			if credential.userID != tt.userID {
				t.Errorf("expected user ID %v, got %v", tt.userID, credential.userID)
			}

			if credential.passwordHash != tt.hash {
				t.Error("credential contains incorrect password hash")
			}
		})
	}
}

func TestPasswordCredential_ChangePasswordHash(t *testing.T) {
	tests := []struct {
		name     string
		newHash  string
		wantHash string
		wantErr  error
	}{
		{
			name:     "success",
			newHash:  "new-hash",
			wantHash: "new-hash",
		},
		{
			name:     "empty hash",
			newHash:  "",
			wantHash: "old-hash",
			wantErr:  ErrInvalidPasswordHash,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credential := &PasswordCredential{
				userID:       uuid.New(),
				passwordHash: "old-hash",
			}

			err := credential.changePasswordHash(tt.newHash)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}

			if credential.passwordHash != tt.wantHash {
				t.Errorf("expected hash %q, got %q", tt.wantHash, credential.passwordHash)
			}
		})
	}
}

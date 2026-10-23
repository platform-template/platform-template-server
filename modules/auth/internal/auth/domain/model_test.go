package domain

import (
	"testing"

	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

			require.ErrorIs(t, err, tt.wantErr)

			if tt.wantErr != nil {
				assert.Nil(t, credential)
				return
			}

			require.NotNil(t, credential)

			assert.Equal(t, tt.userID, credential.userID)

			assert.Equal(t, tt.hash, credential.passwordHash)
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
			name:     testCaseSuccess,
			newHash:  testNewHash,
			wantHash: testNewHash,
		},
		{
			name:     "empty hash",
			newHash:  "",
			wantHash: testOldHash,
			wantErr:  ErrInvalidPasswordHash,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			credential := &PasswordCredential{
				userID:       uuid.New(),
				passwordHash: testOldHash,
			}

			err := credential.changePasswordHash(tt.newHash)

			require.ErrorIs(t, err, tt.wantErr)

			assert.Equal(t, tt.wantHash, credential.passwordHash)
		})
	}
}

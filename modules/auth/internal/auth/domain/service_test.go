package domain

import (
	"errors"
	"testing"

	"uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeAuthRepository struct {
	credential *PasswordCredential
	storeErr   error
	changeErr  error

	stored      *PasswordCredential
	changed     *PasswordCredential
	storeCalls  int
	changeCalls int
}

func (f *fakeAuthRepository) FindByUserID(_ uuid.UUID) (*PasswordCredential, error) {
	return f.credential, nil
}

func (f *fakeAuthRepository) Store(credential *PasswordCredential) error {
	f.storeCalls++
	f.stored = credential
	return f.storeErr
}

func (f *fakeAuthRepository) Change(credential *PasswordCredential) error {
	f.changeCalls++
	f.changed = credential
	return f.changeErr
}

var _ AuthRepository = (*fakeAuthRepository)(nil)

type fakePasswordHasher struct {
	hashResult   string
	verifyResult bool

	hashCalls        int
	hashedPassword   string
	verifiedPassword string
	verifiedHash     string
}

func (f *fakePasswordHasher) Hash(password string) (string, error) {
	f.hashCalls++
	f.hashedPassword = password
	return f.hashResult, nil
}

func (f *fakePasswordHasher) Verify(password, hash string) (bool, error) {
	f.verifiedPassword = password
	f.verifiedHash = hash
	return f.verifyResult, nil
}

var _ PasswordHasher = (*fakePasswordHasher)(nil)

func TestAuthService_Create(t *testing.T) {
	storeErr := errors.New("storage failed")
	tests := []struct {
		name     string
		password string
		existing bool
		storeErr error
		wantErr  error
	}{
		{
			name:     testCaseSuccess,
			password: testPassword,
		},
		{
			name:     "invalid password",
			password: "short",
			wantErr:  ErrInvalidPassword,
		},
		{
			name:     "credential exists",
			password: testPassword,
			existing: true,
			wantErr:  ErrCredentialAlreadyExists,
		},
		{
			name:     "store error",
			password: testPassword,
			storeErr: storeErr,
			wantErr:  storeErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID := uuid.New()
			repo := &fakeAuthRepository{storeErr: tt.storeErr}
			if tt.existing {
				repo.credential = &PasswordCredential{
					userID:       userID,
					passwordHash: testOldHash,
				}
			}
			hasher := &fakePasswordHasher{hashResult: "created-hash"}
			service := NewAuthService(repo, hasher)

			err := service.Create(userID, tt.password)
			require.ErrorIs(t, err, tt.wantErr)

			if tt.wantErr != nil {
				if tt.storeErr == nil {
					assert.Zero(t, repo.storeCalls, "invalid or duplicate credentials must not be stored")
				}
				return
			}

			require.NotNil(t, repo.stored)
			assert.Equal(t, userID, repo.stored.userID)
			assert.Equal(t, "created-hash", repo.stored.passwordHash)
			assert.Equal(t, tt.password, hasher.hashedPassword)
		})
	}
}

func TestAuthService_Check(t *testing.T) {
	tests := []struct {
		name        string
		missing     bool
		verifyMatch bool
		wantMatch   bool
		wantErr     error
	}{
		{
			name:        "correct password",
			verifyMatch: true,
			wantMatch:   true,
		},
		{
			name: "wrong password",
		},
		{
			name:    "credential not found",
			missing: true,
			wantErr: ErrCredentialNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID := uuid.New()
			repo := &fakeAuthRepository{}
			if !tt.missing {
				repo.credential = &PasswordCredential{
					userID:       userID,
					passwordHash: "stored-hash",
				}
			}
			hasher := &fakePasswordHasher{verifyResult: tt.verifyMatch}
			service := NewAuthService(repo, hasher)

			match, err := service.Check(userID, "entered-password")
			require.ErrorIs(t, err, tt.wantErr)
			assert.Equal(t, tt.wantMatch, match)
			if !tt.missing {
				assert.Equal(t, "entered-password", hasher.verifiedPassword)
				assert.Equal(t, "stored-hash", hasher.verifiedHash)
			}
		})
	}
}

func TestAuthService_ChangePassword(t *testing.T) {
	changeErr := errors.New("update failed")
	tests := []struct {
		name        string
		missing     bool
		verifyMatch bool
		changeErr   error
		wantErr     error
	}{
		{
			name: testCaseSuccess,
		},
		{
			name:        "password unchanged",
			verifyMatch: true,
			wantErr:     ErrPasswordUnchanged,
		},
		{
			name:    "credential not found",
			missing: true,
			wantErr: ErrCredentialNotFound,
		},
		{
			name:      "change error",
			changeErr: changeErr,
			wantErr:   changeErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userID := uuid.New()
			repo := &fakeAuthRepository{changeErr: tt.changeErr}
			if !tt.missing {
				repo.credential = &PasswordCredential{
					userID:       userID,
					passwordHash: testOldHash,
				}
			}
			hasher := &fakePasswordHasher{
				hashResult:   testNewHash,
				verifyResult: tt.verifyMatch,
			}
			service := NewAuthService(repo, hasher)

			err := service.ChangePassword(userID, "new-password-123")
			require.ErrorIs(t, err, tt.wantErr)

			if tt.verifyMatch {
				assert.Zero(t, repo.changeCalls, "unchanged password must not be saved")
				assert.Zero(t, hasher.hashCalls, "unchanged password must not be rehashed")
				assert.Equal(t, testOldHash, repo.credential.passwordHash)
			}

			if tt.wantErr != nil {
				return
			}
			require.NotNil(t, repo.changed)
			assert.Equal(t, testNewHash, repo.changed.passwordHash)
			assert.Equal(t, "new-password-123", hasher.verifiedPassword)
			assert.Equal(t, testOldHash, hasher.verifiedHash)
		})
	}
}

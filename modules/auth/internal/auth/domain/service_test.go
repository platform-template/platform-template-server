package domain

import (
	"errors"
	"testing"

	"uuid"
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
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("want error %v, got %v", tt.wantErr, err)
			}

			if tt.wantErr != nil {
				if tt.storeErr == nil && repo.storeCalls != 0 {
					t.Error("invalid or duplicate credentials must not be stored")
				}
				return
			}

			if repo.stored == nil {
				t.Fatal("credential was not stored")
			}
			if repo.stored.userID != userID || repo.stored.passwordHash != "created-hash" {
				t.Error("stored credential has incorrect data")
			}
			if hasher.hashedPassword != tt.password {
				t.Error("hasher received an unexpected password")
			}
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
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("want error %v, got %v", tt.wantErr, err)
			}
			if match != tt.wantMatch {
				t.Errorf("want match=%t, got %t", tt.wantMatch, match)
			}
			if !tt.missing && (hasher.verifiedPassword != "entered-password" || hasher.verifiedHash != "stored-hash") {
				t.Error("Verify must receive the entered password and stored hash")
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
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("want error %v, got %v", tt.wantErr, err)
			}

			if tt.verifyMatch {
				if repo.changeCalls != 0 || hasher.hashCalls != 0 {
					t.Error("unchanged password must not be rehashed or saved")
				}
				if repo.credential.passwordHash != testOldHash {
					t.Error("old hash should be preserved")
				}
			}

			if tt.wantErr != nil {
				return
			}
			if repo.changed == nil || repo.changed.passwordHash != testNewHash {
				t.Error("new password hash was not saved")
			}
			if hasher.verifiedPassword != "new-password-123" || hasher.verifiedHash != testOldHash {
				t.Error("Verify must compare new password with old hash")
			}
		})
	}
}

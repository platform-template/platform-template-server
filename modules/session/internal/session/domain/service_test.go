package domain

import (
	"errors"
	"testing"
	"time"

	"uuid"
)

type fakeSessionRepository struct {
	nextID       uuid.UUID
	nextIDErr    error
	storeErr     error
	findErr      error
	revokeErr    error
	revokeAllErr error

	session        *Session
	storedSession  *Session
	receivedHash   SessionTokenHash
	receivedUserID uuid.UUID

	nextIDCalls    int
	storeCalls     int
	findCalls      int
	revokeCalls    int
	revokeAllCalls int
}

func (f *fakeSessionRepository) NextID() (uuid.UUID, error) {
	f.nextIDCalls++
	return f.nextID, f.nextIDErr
}

func (f *fakeSessionRepository) Store(session *Session) error {
	f.storeCalls++
	f.storedSession = session
	return f.storeErr
}

func (f *fakeSessionRepository) FindByTokenHash(hash SessionTokenHash) (*Session, error) {
	f.findCalls++
	f.receivedHash = hash
	return f.session, f.findErr
}

func (f *fakeSessionRepository) RevokeByTokenHash(hash SessionTokenHash) error {
	f.revokeCalls++
	f.receivedHash = hash
	return f.revokeErr
}

func (f *fakeSessionRepository) RevokeAllByUserID(userID uuid.UUID) error {
	f.revokeAllCalls++
	f.receivedUserID = userID
	return f.revokeAllErr
}

var _ SessionRepository = (*fakeSessionRepository)(nil)

func TestSessionService_Create(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		userID := uuid.New()
		sessionID := uuid.New()

		repo := &fakeSessionRepository{nextID: sessionID}
		service := NewSessionService(repo)

		token, err := service.Create(userID)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if token == "" {
			t.Fatal("expected non-empty token")
		}

		if repo.nextIDCalls != 1 || repo.storeCalls != 1 {
			t.Errorf("expected one NextID and Store call, got %d and %d", repo.nextIDCalls, repo.storeCalls)
		}

		if repo.storedSession == nil {
			t.Fatal("session was not stored")
		}

		if repo.storedSession.id != sessionID {
			t.Errorf("expected session ID %v, got %v", sessionID, repo.storedSession.id)
		}

		if repo.storedSession.userID != userID {
			t.Errorf("expected user ID %v, got %v", userID, repo.storedSession.userID)
		}

		if repo.storedSession.tokenHash != HashSessionToken(token) {
			t.Error("stored token hash does not match returned token")
		}
	})

	t.Run("invalid user ID", func(t *testing.T) {
		repo := &fakeSessionRepository{}
		service := NewSessionService(repo)

		token, err := service.Create(uuid.Nil())

		if !errors.Is(err, ErrInvalidUserID) {
			t.Fatalf("expected ErrInvalidUserID, got %v", err)
		}

		if token != "" {
			t.Errorf("expected empty token, got %q", token)
		}

		if repo.nextIDCalls != 0 || repo.storeCalls != 0 {
			t.Error("repository should not be called for invalid user ID")
		}
	})

	t.Run("next ID error", func(t *testing.T) {
		repoErr := errors.New("next ID failed")

		repo := &fakeSessionRepository{nextIDErr: repoErr}
		service := NewSessionService(repo)

		token, err := service.Create(uuid.New())

		if !errors.Is(err, repoErr) {
			t.Fatalf("expected NextID error, got %v", err)
		}

		if token != "" {
			t.Errorf("expected empty token, got %q", token)
		}

		if repo.storeCalls != 0 {
			t.Error("Store should not be called after NextID failure")
		}
	})

	t.Run("store error", func(t *testing.T) {
		repoErr := errors.New("store failed")

		repo := &fakeSessionRepository{
			nextID:   uuid.New(),
			storeErr: repoErr,
		}
		service := NewSessionService(repo)

		token, err := service.Create(uuid.New())

		if !errors.Is(err, repoErr) {
			t.Fatalf("expected Store error, got %v", err)
		}

		if token != "" {
			t.Errorf("expected empty token, got %q", token)
		}

		if repo.storeCalls != 1 {
			t.Errorf("expected one Store call, got %d", repo.storeCalls)
		}
	})
}

func TestSessionService_Validate(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		token     SessionToken
		session   *Session
		repoErr   error
		wantValid bool
		wantErr   error
		wantCalls int
	}{
		{
			name:      "valid session",
			token:     "valid-token",
			session:   &Session{expiresAt: now.Add(time.Hour)},
			wantValid: true,
			wantCalls: 1,
		},
		{
			name:      "expired session",
			token:     "expired-token",
			session:   &Session{expiresAt: now.Add(-time.Hour)},
			wantValid: false,
			wantCalls: 1,
		},
		{
			name:      "empty token",
			token:     "",
			wantErr:   ErrInvalidToken,
			wantCalls: 0,
		},
		{
			name:      "repository error",
			token:     "some-token",
			repoErr:   errors.New("database unavailable"),
			wantValid: false,
			wantCalls: 1,
		},
		{
			name:      "session not found",
			token:     "unknown-token",
			repoErr:   errors.New("session not found"),
			wantValid: false,
			wantCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeSessionRepository{
				session: tt.session,
				findErr: tt.repoErr,
			}
			service := NewSessionService(repo)

			valid, err := service.Validate(tt.token)

			wantErr := tt.wantErr
			if tt.repoErr != nil {
				wantErr = tt.repoErr
			}

			if !errors.Is(err, wantErr) {
				t.Fatalf("expected error %v, got %v", wantErr, err)
			}

			if valid != tt.wantValid {
				t.Errorf("expected valid=%t, got %t", tt.wantValid, valid)
			}

			if repo.findCalls != tt.wantCalls {
				t.Errorf("expected %d Find calls, got %d", tt.wantCalls, repo.findCalls)
			}

			if tt.token != "" && repo.receivedHash != HashSessionToken(tt.token) {
				t.Error("repository received incorrect token hash")
			}
		})
	}
}

func TestSessionService_RevokeByToken(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		token := SessionToken("some-token")

		repo := &fakeSessionRepository{}
		service := NewSessionService(repo)

		err := service.RevokeByToken(token)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if repo.revokeCalls != 1 {
			t.Errorf("expected one Revoke call, got %d", repo.revokeCalls)
		}

		if repo.receivedHash != HashSessionToken(token) {
			t.Error("repository received incorrect token hash")
		}
	})

	t.Run("empty token", func(t *testing.T) {
		repo := &fakeSessionRepository{}
		service := NewSessionService(repo)

		err := service.RevokeByToken("")

		if !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("expected ErrInvalidToken, got %v", err)
		}

		if repo.revokeCalls != 0 {
			t.Error("repository should not be called for empty token")
		}
	})

	t.Run("repository error", func(t *testing.T) {
		repoErr := errors.New("revoke failed")

		repo := &fakeSessionRepository{revokeErr: repoErr}
		service := NewSessionService(repo)

		err := service.RevokeByToken("some-token")

		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repository error, got %v", err)
		}

		if repo.revokeCalls != 1 {
			t.Errorf("expected one Revoke call, got %d", repo.revokeCalls)
		}
	})
}

func TestSessionService_RevokeAllByUserID(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		userID := uuid.New()

		repo := &fakeSessionRepository{}
		service := NewSessionService(repo)

		err := service.RevokeAllByUserID(userID)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if repo.revokeAllCalls != 1 {
			t.Errorf("expected one RevokeAll call, got %d", repo.revokeAllCalls)
		}

		if repo.receivedUserID != userID {
			t.Errorf("expected user ID %v, got %v", userID, repo.receivedUserID)
		}
	})

	t.Run("invalid user ID", func(t *testing.T) {
		repo := &fakeSessionRepository{}
		service := NewSessionService(repo)

		err := service.RevokeAllByUserID(uuid.Nil())

		if !errors.Is(err, ErrInvalidUserID) {
			t.Fatalf("expected ErrInvalidUserID, got %v", err)
		}

		if repo.revokeAllCalls != 0 {
			t.Error("repository should not be called for invalid user ID")
		}
	})

	t.Run("repository error", func(t *testing.T) {
		repoErr := errors.New("revoke all failed")

		repo := &fakeSessionRepository{revokeAllErr: repoErr}
		service := NewSessionService(repo)

		err := service.RevokeAllByUserID(uuid.New())

		if !errors.Is(err, repoErr) {
			t.Fatalf("expected repository error, got %v", err)
		}

		if repo.revokeAllCalls != 1 {
			t.Errorf("expected one RevokeAll call, got %d", repo.revokeAllCalls)
		}
	})
}

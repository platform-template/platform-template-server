package domain

import (
	"errors"
	"testing"
	"time"

	"uuid"
)

const testToken = SessionToken("token")

type fakeSessionRepository struct {
	nextID                                        uuid.UUID
	storeErr, findErr, revokeErr, revokeOthersErr error
	session                                       *Session
	stored                                        *Session
	findHash, revokedHash, excludedHash           SessionTokenHash
	revokedUserID                                 uuid.UUID
	revokeOthersCalls                             int
}

func (r *fakeSessionRepository) NextID() (uuid.UUID, error) {
	return r.nextID, nil
}
func (r *fakeSessionRepository) Store(s *Session) error {
	r.stored = s
	return r.storeErr
}
func (r *fakeSessionRepository) FindByTokenHash(hash SessionTokenHash) (*Session, error) {
	r.findHash = hash
	return r.session, r.findErr
}
func (r *fakeSessionRepository) RevokeByTokenHash(hash SessionTokenHash) error {
	r.revokedHash = hash
	return r.revokeErr
}
func (r *fakeSessionRepository) RevokeOthersByUserID(userID uuid.UUID, hash SessionTokenHash) error {
	r.revokeOthersCalls++
	r.revokedUserID, r.excludedHash = userID, hash
	return r.revokeOthersErr
}

func TestSessionServiceCreateStoresOnlyTokenHash(t *testing.T) {
	userID := uuid.New()
	repo := &fakeSessionRepository{nextID: uuid.New()}
	token, err := NewSessionService(repo).Create(userID)
	if err != nil {
		t.Fatal(err)
	}
	if token == "" || repo.stored == nil || repo.stored.Data().UserID != userID {
		t.Fatal("session was not stored")
	}
	if repo.stored.Data().TokenHash != HashSessionToken(token) {
		t.Fatal("stored hash does not match returned token")
	}
	if string(repo.stored.Data().TokenHash) == string(token) {
		t.Fatal("plain token must not be persisted")
	}
}

func TestSessionServiceResolve(t *testing.T) {
	userID := uuid.New()
	tests := []struct {
		name    string
		token   SessionToken
		session *Session
		wantID  uuid.UUID
		wantErr error
	}{
		{
			name:  "active session",
			token: testToken,
			session: &Session{
				userID:    userID,
				expiresAt: time.Now().Add(time.Hour),
			},
			wantID: userID,
		},
		{
			name:    "unknown token",
			token:   testToken,
			wantErr: ErrSessionNotFound,
		},
		{
			name:  "expired session",
			token: testToken,
			session: &Session{
				expiresAt: time.Now().Add(-time.Hour),
			},
			wantErr: ErrSessionExpired,
		},
		{
			name:    "empty token",
			wantErr: ErrInvalidToken,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fakeSessionRepository{session: tt.session}
			got, err := NewSessionService(repo).Resolve(tt.token)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
			if got != tt.wantID {
				t.Fatalf("expected user %v, got %v", tt.wantID, got)
			}
			if tt.token != "" && repo.findHash != HashSessionToken(tt.token) {
				t.Fatal("lookup must use token hash")
			}
		})
	}
}

func TestSessionServiceRevokeByToken(t *testing.T) {
	repo := &fakeSessionRepository{}
	if err := NewSessionService(repo).RevokeByToken("token"); err != nil {
		t.Fatal(err)
	}
	if repo.revokedHash != HashSessionToken("token") {
		t.Fatal("revocation must use token hash")
	}
	if err := NewSessionService(repo).RevokeByToken(""); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected invalid token, got %v", err)
	}
}

func TestSessionServiceRevokeOthersKeepsCurrentValidSession(t *testing.T) {
	userID := uuid.New()
	token := SessionToken("current-token")
	repo := &fakeSessionRepository{
		session: &Session{
			userID:    userID,
			expiresAt: time.Now().Add(time.Hour),
		},
	}
	if err := NewSessionService(repo).RevokeOthersByUserID(userID, token); err != nil {
		t.Fatal(err)
	}
	if repo.revokeOthersCalls != 1 ||
		repo.revokedUserID != userID ||
		repo.excludedHash != HashSessionToken(token) {
		t.Fatal("current token was not excluded from revocation")
	}
}

func TestSessionServiceRevokeOthersRejectsForeignToken(t *testing.T) {
	userID := uuid.New()
	repo := &fakeSessionRepository{
		session: &Session{
			userID:    uuid.New(),
			expiresAt: time.Now().Add(time.Hour),
		},
	}
	err := NewSessionService(repo).RevokeOthersByUserID(userID, "foreign-token")
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected invalid token, got %v", err)
	}
	if repo.revokeOthersCalls != 0 {
		t.Fatal("must not revoke sessions for a foreign token")
	}
}

package domain

import (
	"testing"
	"time"

	"uuid"

	"github.com/stretchr/testify/require"
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
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.NotNil(t, repo.stored)
	require.Equal(t, userID, repo.stored.Data().UserID)
	require.Equal(t, HashSessionToken(token), repo.stored.Data().TokenHash)
	require.NotEqual(t, string(token), string(repo.stored.Data().TokenHash))
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
			require.ErrorIs(t, err, tt.wantErr)
			require.Equal(t, tt.wantID, got)
			if tt.token != "" {
				require.Equal(t, HashSessionToken(tt.token), repo.findHash)
			}
		})
	}
}

func TestSessionServiceRevokeByToken(t *testing.T) {
	repo := &fakeSessionRepository{}
	err := NewSessionService(repo).RevokeByToken("token")
	require.NoError(t, err)
	require.Equal(t, HashSessionToken("token"), repo.revokedHash)
	err = NewSessionService(repo).RevokeByToken("")
	require.ErrorIs(t, err, ErrInvalidToken)
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
	err := NewSessionService(repo).RevokeOthersByUserID(userID, token)
	require.NoError(t, err)
	require.Equal(t, 1, repo.revokeOthersCalls)
	require.Equal(t, userID, repo.revokedUserID)
	require.Equal(t, HashSessionToken(token), repo.excludedHash)
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
	require.ErrorIs(t, err, ErrInvalidToken)
	require.Equal(t, 0, repo.revokeOthersCalls)
}

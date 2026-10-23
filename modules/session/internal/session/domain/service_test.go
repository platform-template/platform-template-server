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
	findToken, revokedToken, excludedToken        SessionToken
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
func (r *fakeSessionRepository) FindByToken(token SessionToken) (*Session, error) {
	r.findToken = token
	return r.session, r.findErr
}
func (r *fakeSessionRepository) RevokeByToken(token SessionToken) error {
	r.revokedToken = token
	return r.revokeErr
}
func (r *fakeSessionRepository) RevokeOthersByUserID(userID uuid.UUID, token SessionToken) error {
	r.revokeOthersCalls++
	r.revokedUserID, r.excludedToken = userID, token
	return r.revokeOthersErr
}

func TestSessionServiceCreateStoresToken(t *testing.T) {
	userID := uuid.New()
	repo := &fakeSessionRepository{nextID: uuid.New()}
	token, err := NewSessionService(repo).Create(userID)
	require.NoError(t, err)
	require.NotEmpty(t, token)
	require.NotNil(t, repo.stored)
	require.Equal(t, userID, repo.stored.Data().UserID)
	require.Equal(t, token, repo.stored.Data().Token)
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
				require.Equal(t, tt.token, repo.findToken)
			}
		})
	}
}

func TestSessionServiceRevokeByToken(t *testing.T) {
	repo := &fakeSessionRepository{}
	err := NewSessionService(repo).RevokeByToken("token")
	require.NoError(t, err)
	require.Equal(t, SessionToken("token"), repo.revokedToken)
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
	require.Equal(t, token, repo.excludedToken)
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

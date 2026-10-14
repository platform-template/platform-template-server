package domain

import (
	"errors"
	"fmt"
	"time"
	"uuid"
)

const SessionTTL = 1 * 24 * time.Hour

var (
	ErrInvalidUserID   = errors.New("invalid user ID")
	ErrInvalidToken    = errors.New("invalid session token")
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionExpired  = errors.New("session expired")
	ErrStoreSession    = errors.New("store session")
)

type SessionService struct {
	repo SessionRepository
}

func NewSessionService(repo SessionRepository) *SessionService {
	return &SessionService{repo: repo}
}

func (s *SessionService) Create(userID uuid.UUID) (SessionToken, error) {
	if userID == uuid.Nil() {
		return "", fmt.Errorf("create session: %w", ErrInvalidUserID)
	}

	id, err := s.repo.NextID()
	if err != nil {
		return "", fmt.Errorf("create session: generate ID: %w", err)
	}

	token, err := GenerateSessionToken()
	if err != nil {
		return "", fmt.Errorf("create session: generate token: %w", err)
	}

	session, err := NewSession(id, userID, HashSessionToken(token), time.Now().UTC().Add(SessionTTL))
	if err != nil {
		return "", fmt.Errorf("create session: new session: %w", err)
	}

	if err := s.repo.Store(session); err != nil {
		return "", fmt.Errorf("create session: %w: %w", ErrStoreSession, err)
	}

	return token, nil
}

func (s *SessionService) Resolve(token SessionToken) (uuid.UUID, error) {
	if token == "" {
		return uuid.Nil(), fmt.Errorf("resolve session: %w", ErrInvalidToken)
	}

	session, err := s.repo.FindByTokenHash(HashSessionToken(token))
	if err != nil {
		return uuid.Nil(), fmt.Errorf("resolve session: find by token hash: %w", err)
	}
	if session == nil {
		return uuid.Nil(), fmt.Errorf("resolve session: %w", ErrSessionNotFound)
	}
	if session.IsExpired(time.Now()) {
		return uuid.Nil(), fmt.Errorf("resolve session: %w", ErrSessionExpired)
	}

	return session.userID, nil
}

func (s *SessionService) RevokeByToken(token SessionToken) error {
	if token == "" {
		return fmt.Errorf("revoke session: %w", ErrInvalidToken)
	}

	if err := s.repo.RevokeByTokenHash(HashSessionToken(token)); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}

	return nil
}

func (s *SessionService) RevokeOthersByUserID(userID uuid.UUID, token SessionToken) error {
	if userID == uuid.Nil() {
		return fmt.Errorf("revoke other sessions: %w", ErrInvalidUserID)
	}

	currentUserID, err := s.Resolve(token)
	if err != nil {
		return fmt.Errorf("revoke other sessions: resolve token: %w", err)
	}
	if currentUserID != userID {
		return fmt.Errorf("revoke other sessions: %w", ErrInvalidToken)
	}

	tokenHash := HashSessionToken(token)
	if err := s.repo.RevokeOthersByUserID(userID, tokenHash); err != nil {
		return fmt.Errorf("revoke other sessions: %w", err)
	}

	return nil
}

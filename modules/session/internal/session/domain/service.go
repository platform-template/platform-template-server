package domain

import (
	"errors"
	"fmt"
	"uuid"

	"github.com/distributed-programming-2026/lib/event"
)

var (
	ErrInvalidUserID   = errors.New("invalid user ID")
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionExpired  = errors.New("session expired")
)

type SessionService struct {
	repo       SessionRepository
	dispatcher event.Dispatcher
}

func NewSessionService(repo SessionRepository, dispatcher event.Dispatcher) *SessionService {
	return &SessionService{
		repo:       repo,
		dispatcher: dispatcher,
	}
}

func (s *SessionService) Create(userID uuid.UUID) (SessionToken, error) {
	if userID == uuid.Nil() {
		return "", ErrInvalidUserID
	}

	id, err := s.repo.NextID()
	if err != nil {
		return "", fmt.Errorf("create session: generate ID: %w", err)
	}

	token, err := GenerateSessionToken()
	if err != nil {
		return "", fmt.Errorf("create session: generate session token: %w", err)
	}

	tokenHash := HashSessionToken(token)

	session := Session{
		id:        id,
		userID:    userID,
		tokenHash: tokenHash,
	}

	err = s.repo.Store(&session)
	if err != nil {
		return "", fmt.Errorf("create session: store session: %w", err)
	}

	return token, nil
}

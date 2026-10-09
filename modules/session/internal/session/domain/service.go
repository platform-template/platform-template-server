package domain

import (
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/distributed-programming-2026/lib/event"
)

var (
	ErrInvalidUserID = errors.New("invalid user ID")
	ErrInvalidToken  = errors.New("invalid user token")
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
		return "", fmt.Errorf("create session: %w", ErrInvalidUserID)
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

func (s *SessionService) Validate(token SessionToken) (bool, error) {
	if token == "" {
		return false, fmt.Errorf("validate session: %w", ErrInvalidToken)
	}

	tokenHash := HashSessionToken(token)

	session, err := s.repo.FindByTokenHash(tokenHash)
	if err != nil {
		return false, fmt.Errorf("validate session: find by token hash: %w", err)
	}

	return !session.IsExpired(time.Now()), nil
}

func (s *SessionService) RevokeByToken(token SessionToken) error {
	if token == "" {
		return fmt.Errorf("revoke session: %w", ErrInvalidToken)
	}

	tokenHash := HashSessionToken(token)

	err := s.repo.RevokeByTokenHash(tokenHash)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}

	return nil
}

func (s *SessionService) RevokeAllByUserID(userID uuid.UUID) error {
	if userID == uuid.Nil() {
		return fmt.Errorf("revoke all sessions: %w", ErrInvalidUserID)
	}

	err := s.repo.RevokeAllByUserID(userID)
	if err != nil {
		return fmt.Errorf("revoke all sessions: %w", err)
	}

	return nil
}

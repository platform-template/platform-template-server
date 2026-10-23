package domain

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
	"uuid"
)

type SessionToken string

type Session struct {
	id        uuid.UUID
	userID    uuid.UUID
	token     SessionToken
	expiresAt time.Time
}

type SessionData struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Token     SessionToken
	ExpiresAt time.Time
}

var ErrInvalidSession = errors.New("invalid session data")

func NewSession(id, userID uuid.UUID, token SessionToken, expiresAt time.Time) (*Session, error) {
	if id == uuid.Nil() || userID == uuid.Nil() || token == "" || expiresAt.IsZero() {
		return nil, ErrInvalidSession
	}

	return &Session{
		id:        id,
		userID:    userID,
		token:     token,
		expiresAt: expiresAt,
	}, nil
}

func (s *Session) Data() SessionData {
	return SessionData{
		ID:        s.id,
		UserID:    s.userID,
		Token:     s.token,
		ExpiresAt: s.expiresAt,
	}
}

type SessionRepository interface {
	NextID() (uuid.UUID, error)
	Store(*Session) error
	FindByToken(SessionToken) (*Session, error)
	RevokeByToken(SessionToken) error
	RevokeOthersByUserID(uuid.UUID, SessionToken) error
}

func (s *Session) IsExpired(now time.Time) bool {
	return !now.Before(s.expiresAt)
}

func GenerateSessionToken() (SessionToken, error) {
	bytes := make([]byte, 32)

	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}

	return SessionToken(base64.RawURLEncoding.EncodeToString(bytes)), nil
}

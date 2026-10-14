package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"
	"uuid"
)

type SessionToken string
type SessionTokenHash string

type Session struct {
	id        uuid.UUID
	userID    uuid.UUID
	tokenHash SessionTokenHash
	expiresAt time.Time
}

type SessionData struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	TokenHash SessionTokenHash
	ExpiresAt time.Time
}

var ErrInvalidSession = errors.New("invalid session data")

func NewSession(id, userID uuid.UUID, tokenHash SessionTokenHash, expiresAt time.Time) (*Session, error) {
	if id == uuid.Nil() || userID == uuid.Nil() || tokenHash == "" || expiresAt.IsZero() {
		return nil, ErrInvalidSession
	}

	return &Session{
		id:        id,
		userID:    userID,
		tokenHash: tokenHash,
		expiresAt: expiresAt,
	}, nil
}

func (s *Session) Data() SessionData {
	return SessionData{
		ID:        s.id,
		UserID:    s.userID,
		TokenHash: s.tokenHash,
		ExpiresAt: s.expiresAt,
	}
}

type SessionRepository interface {
	NextID() (uuid.UUID, error)
	Store(*Session) error
	FindByTokenHash(SessionTokenHash) (*Session, error)
	RevokeByTokenHash(SessionTokenHash) error
	RevokeOthersByUserID(uuid.UUID, SessionTokenHash) error
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

func HashSessionToken(token SessionToken) SessionTokenHash {
	hash := sha256.Sum256([]byte(token))
	return SessionTokenHash(hex.EncodeToString(hash[:]))
}

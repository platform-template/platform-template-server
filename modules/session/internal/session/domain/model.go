package domain

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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

type SessionRepository interface {
	NextID() (uuid.UUID, error)
	Store(s *Session) error
	Revoke(s *Session) error
	FindByTokenHash(token SessionTokenHash) (*Session, error)
	RevokeByTokenHash(token SessionTokenHash) error
	RevokeAllByUserID(userID uuid.UUID) ([]Session, error)
}

func (s *Session) IsExpired() bool {
	return !time.Now().Before(s.expiresAt)
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

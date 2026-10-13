package password

import (
	"fmt"

	"github.com/alexedwards/argon2id"

	"auth/internal/auth/domain"
)

type Argon2Hasher struct{}

func NewArgon2Hasher() *Argon2Hasher {
	return &Argon2Hasher{}
}

var _ domain.PasswordHasher = (*Argon2Hasher)(nil)

func (h *Argon2Hasher) Hash(password string) (string, error) {
	params := &argon2id.Params{
		Memory:      7 * 1024,
		Iterations:  5,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	}

	hash, err := argon2id.CreateHash(password, params)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}

	return hash, nil
}

func (h *Argon2Hasher) Verify(password, hash string) (bool, error) {
	match, err := argon2id.ComparePasswordAndHash(password, hash)
	if err != nil {
		return false, fmt.Errorf("verify password: %w", err)
	}

	return match, nil
}

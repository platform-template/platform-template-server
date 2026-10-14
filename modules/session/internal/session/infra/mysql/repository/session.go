package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"

	"session/internal/session/domain"
)

type sessionRepository struct {
	ctx    context.Context
	client mysql.ClientContext
}

func NewSessionRepository(ctx context.Context, client mysql.ClientContext) domain.SessionRepository {
	return &sessionRepository{
		ctx:    ctx,
		client: client,
	}
}

func (r *sessionRepository) NextID() (uuid.UUID, error) {
	return uuid.New(), nil
}

func (r *sessionRepository) Store(session *domain.Session) error {
	data := session.Data()

	_, err := r.client.ExecContext(r.ctx, "session.store", `
INSERT INTO sessions (id, user_id, token_hash, expires_at)
VALUES (?, ?, ?, ?)
`,
		data.ID.String(),
		data.UserID.String(),
		string(data.TokenHash),
		data.ExpiresAt)

	if err != nil {
		return fmt.Errorf("store session: %w", err)
	}

	return nil
}

func (r *sessionRepository) FindByTokenHash(tokenHash domain.SessionTokenHash) (*domain.Session, error) {
	var id, userID, storedHash string
	var expiresAt time.Time

	err := r.client.QueryRowContext(r.ctx, "session.find_by_token_hash", `
SELECT id, user_id, token_hash, expires_at
FROM sessions
WHERE token_hash = ?
`, string(tokenHash)).Scan(&id, &userID, &storedHash, &expiresAt)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find session by token hash: %w", err)
	}

	sessionID, err := uuid.Parse(id)
	if err != nil {
		return nil, fmt.Errorf("parse session ID: %w", err)
	}

	parsedUserID, err := uuid.Parse(userID)
	if err != nil {
		return nil, fmt.Errorf("parse user ID: %w", err)
	}

	session, err := domain.NewSession(
		sessionID,
		parsedUserID,
		domain.SessionTokenHash(storedHash),
		expiresAt,
	)
	if err != nil {
		return nil, fmt.Errorf("restore session: %w", err)
	}

	return session, nil
}

func (r *sessionRepository) RevokeByTokenHash(tokenHash domain.SessionTokenHash) error {
	_, err := r.client.ExecContext(r.ctx, "session.revoke_by_token_hash", `
DELETE FROM sessions WHERE token_hash = ?
`, string(tokenHash))

	if err != nil {
		return fmt.Errorf("revoke session by token hash: %w", err)
	}

	return nil
}

func (r *sessionRepository) RevokeOthersByUserID(userID uuid.UUID, tokenHash domain.SessionTokenHash) error {
	_, err := r.client.ExecContext(r.ctx, "session.revoke_others_by_user_id", `
DELETE FROM sessions
WHERE user_id = ? AND token_hash <> ?
`, userID.String(), string(tokenHash))

	if err != nil {
		return fmt.Errorf("revoke other sessions by user ID: %w", err)
	}

	return nil
}

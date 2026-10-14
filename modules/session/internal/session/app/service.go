package app

import (
	"context"
	"fmt"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/uow"

	"session/internal/session/domain"
	"session/internal/session/infra/mysql"
)

type Service struct {
	unit uow.UnitOfWorkWithRepositoryProvider[*mysql.RepositoryProvider]
}

func NewService(unit uow.UnitOfWorkWithRepositoryProvider[*mysql.RepositoryProvider]) *Service {
	return &Service{unit: unit}
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID) (string, error) {
	var token domain.SessionToken

	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		svc := domain.NewSessionService(provider.Session(ctx))

		var err error
		token, err = svc.Create(userID)
		return err
	})
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}

	return string(token), nil
}

func (s *Service) Resolve(ctx context.Context, token string) (uuid.UUID, error) {
	var userID uuid.UUID

	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		svc := domain.NewSessionService(provider.Session(ctx))

		var err error
		userID, err = svc.Resolve(domain.SessionToken(token))
		return err
	})
	if err != nil {
		return uuid.Nil(), fmt.Errorf("resolve session: %w", err)
	}

	return userID, nil
}

func (s *Service) RevokeByToken(ctx context.Context, token string) error {
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		svc := domain.NewSessionService(provider.Session(ctx))
		return svc.RevokeByToken(domain.SessionToken(token))
	})
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}

	return nil
}

func (s *Service) RevokeOthersByUserID(ctx context.Context, userID uuid.UUID, token string) error {
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		svc := domain.NewSessionService(provider.Session(ctx))
		return svc.RevokeOthersByUserID(userID, domain.SessionToken(token))
	})
	if err != nil {
		return fmt.Errorf("revoke other sessions: %w", err)
	}

	return nil
}

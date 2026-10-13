package app

import (
	"context"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/uow"

	"auth/internal/auth/domain"
	"auth/internal/auth/infra/mysql"
)

type Service struct {
	unit           uow.UnitOfWorkWithRepositoryProvider[*mysql.RepositoryProvider]
	passwordHasher domain.PasswordHasher
}

func NewService(
	unit uow.UnitOfWorkWithRepositoryProvider[*mysql.RepositoryProvider],
	passwordHasher domain.PasswordHasher,
) *Service {
	return &Service{
		unit:           unit,
		passwordHasher: passwordHasher,
	}
}

func (s *Service) Create(ctx context.Context, userID uuid.UUID, password string) error {
	return s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		svc := domain.NewAuthService(provider.Auth(ctx), s.passwordHasher)
		return svc.Create(userID, password)
	})
}

func (s *Service) Check(ctx context.Context, userID uuid.UUID, password string) (bool, error) {
	var match bool

	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		svc := domain.NewAuthService(provider.Auth(ctx), s.passwordHasher)

		var err error
		match, err = svc.Check(userID, password)
		return err
	})

	return match, err
}

func (s *Service) ChangePassword(ctx context.Context, userID uuid.UUID, password string) error {
	return s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		svc := domain.NewAuthService(provider.Auth(ctx), s.passwordHasher)
		return svc.ChangePassword(userID, password)
	})
}

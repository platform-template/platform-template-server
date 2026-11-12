package app

import (
	"context"
	"errors"
	"fmt"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/uow"

	"auth/internal/auth/domain"
	"auth/internal/auth/infra/mysql"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
)

type Service struct {
	unit           uow.UnitOfWorkWithRepositoryProvider[*mysql.RepositoryProvider]
	passwordHasher domain.PasswordHasher
	registration   RegistrationRunner
	identity       IdentityClient
	session        SessionClient
}

type RegistrationRunner interface {
	Register(
		ctx context.Context,
		input RegisterInput,
	) (uuid.UUID, error)
}

func NewService(
	unit uow.UnitOfWorkWithRepositoryProvider[*mysql.RepositoryProvider],
	passwordHasher domain.PasswordHasher,
	identity IdentityClient,
	session SessionClient,
	registration RegistrationRunner,
) *Service {
	return &Service{
		unit:           unit,
		passwordHasher: passwordHasher,
		identity:       identity,
		session:        session,
		registration:   registration,
	}
}

type RegisterInput struct {
	FirstName  string
	LastName   string
	MiddleName string
	Login      string
	Email      string
	Password   string
}

func (s *Service) Register(
	ctx context.Context,
	input RegisterInput,
) (uuid.UUID, error) {
	return s.registration.Register(ctx, input)
}

func (s *Service) Login(ctx context.Context, login, password string) (string, error) {
	var (
		userID uuid.UUID
		match  bool
	)
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		svc := domain.NewAuthService(provider.Auth(ctx), s.passwordHasher)
		var err error
		userID, match, err = svc.Check(login, password)
		return err
	})
	if err != nil {
		return "", fmt.Errorf("login: check password: %w", err)
	}

	if !match {
		return "", fmt.Errorf("login: check password: %w", ErrInvalidCredentials)
	}

	token, err := s.session.Create(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("login: create session: %w", err)
	}

	return token, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	err := s.session.RevokeSession(ctx, token)
	if err != nil {
		return fmt.Errorf("logout: revoke by token: %w", err)
	}

	return nil
}

func (s *Service) ChangePassword(
	ctx context.Context,
	token,
	oldPassword,
	newPassword string,
) error {
	if err := domain.ValidatePassword(newPassword); err != nil {
		return fmt.Errorf("change password: validate new password: %w", err)
	}

	userID, err := s.session.Resolve(ctx, token)
	if err != nil {
		return fmt.Errorf("change password: resolve session: %w", err)
	}

	err = s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		svc := domain.NewAuthService(provider.Auth(ctx), s.passwordHasher)

		match, err := svc.CheckByUserID(userID, oldPassword)
		if err != nil {
			return fmt.Errorf("check old password: %w", err)
		}
		if !match {
			return ErrInvalidCredentials
		}

		if err := svc.ChangePassword(userID, newPassword); err != nil {
			return fmt.Errorf("update password: %w", err)
		}

		return nil
	})
	if err != nil {
		return fmt.Errorf("change password: %w", err)
	}

	err = s.session.RevokeOthersByUserID(ctx, userID, token)
	if err != nil {
		return fmt.Errorf("change password: revoke by user ID: %w", err)
	}

	return nil
}

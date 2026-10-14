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
	identity       IdentityClient
	session        SessionClient
}

func NewService(
	unit uow.UnitOfWorkWithRepositoryProvider[*mysql.RepositoryProvider],
	passwordHasher domain.PasswordHasher,
	identity IdentityClient,
	session SessionClient,
) *Service {
	return &Service{
		unit:           unit,
		passwordHasher: passwordHasher,
		identity:       identity,
		session:        session,
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

func (s *Service) Register(ctx context.Context, input RegisterInput) (uuid.UUID, error) {
	if err := domain.ValidatePassword(input.Password); err != nil {
		return uuid.Nil(), fmt.Errorf("register: validate pass: %w", err)
	}

	userID, err := s.identity.Create(
		ctx,
		input.FirstName,
		input.LastName,
		input.MiddleName,
		input.Login,
		input.Email,
	)
	if err != nil {
		return uuid.Nil(), fmt.Errorf("register: create user: %w", err)
	}

	err = s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		svc := domain.NewAuthService(provider.Auth(ctx), s.passwordHasher)
		return svc.Create(userID, input.Password)
	})
	if err != nil {
		return uuid.Nil(), fmt.Errorf("register: create auth cred: %w", err)
	}

	return userID, nil
}

func (s *Service) Login(ctx context.Context, login, password string) (string, error) {
	userID, err := s.identity.FindByLogin(ctx, login)
	if errors.Is(err, ErrUserNotFound) {
		return "", fmt.Errorf("login: %w", ErrInvalidCredentials)
	}
	if err != nil {
		return "", fmt.Errorf("login: find by login: %w", err)
	}

	var match bool
	err = s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		svc := domain.NewAuthService(provider.Auth(ctx), s.passwordHasher)
		var err error
		match, err = svc.Check(userID, password)
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

		match, err := svc.Check(userID, oldPassword)
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

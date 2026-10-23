package app

import (
	"context"
	"fmt"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/uow"

	"identity/internal/identity/domain"
	"identity/internal/identity/infra/mysql"
)

type Service struct {
	unit uow.UnitOfWorkWithRepositoryProvider[*mysql.RepositoryProvider]
}

func NewService(unit uow.UnitOfWorkWithRepositoryProvider[*mysql.RepositoryProvider]) *Service {
	return &Service{unit: unit}
}

func (s *Service) Create(
	ctx context.Context,
	firstName,
	lastName string,
	middleName *string,
	login,
	email string,
) (uuid.UUID, error) {
	var userID uuid.UUID

	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		svc := domain.NewIdentityService(provider.Identity(ctx))

		var err error
		userID, err = svc.Create(firstName, lastName, middleName, login, email)
		return err
	})
	if err != nil {
		return uuid.Nil(), fmt.Errorf("identity: create user: %w", err)
	}

	return userID, nil
}

func (s *Service) FindByLogin(ctx context.Context, login string) (uuid.UUID, error) {
	var userID uuid.UUID

	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		svc := domain.NewIdentityService(provider.Identity(ctx))

		user, err := svc.FindByLogin(login)
		if err != nil {
			return err
		}

		userID = user.Data().ID
		return nil
	})
	if err != nil {
		return uuid.Nil(), fmt.Errorf("identity: find user by login: %w", err)
	}

	return userID, nil
}

func (s *Service) FindByID(ctx context.Context, userID uuid.UUID) (*domain.User, error) {
	var user *domain.User

	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		var err error
		user, err = domain.NewIdentityService(provider.Identity(ctx)).FindByID(userID)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("identity: find user by ID: %w", err)
	}

	return user, nil
}

func (s *Service) Update(
	ctx context.Context,
	userID uuid.UUID,
	firstName,
	lastName string,
	middleName *string,
	email string,
) (*domain.User, error) {
	var user *domain.User

	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		var err error
		user, err = domain.NewIdentityService(provider.Identity(ctx)).Update(
			userID,
			firstName,
			lastName,
			middleName,
			email,
		)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("identity: update user: %w", err)
	}

	return user, nil
}

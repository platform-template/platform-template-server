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
	lastName,
	middleName,
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

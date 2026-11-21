package app

import (
	"context"
	"errors"
	"fmt"
	"time"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/uow"

	"authz/internal/authz/domain"
	"authz/internal/authz/infra/mysql"
)

type Service struct {
	unit uow.UnitOfWorkWithRepositoryProvider[*mysql.RepositoryProvider]
	now  func() time.Time
}

func NewService(
	unit uow.UnitOfWorkWithRepositoryProvider[*mysql.RepositoryProvider],
) *Service {
	return &Service{unit: unit, now: time.Now}
}

func (s *Service) GrantAccess(
	ctx context.Context,
	actorID, targetUserID uuid.UUID,
	rawModuleKey string,
) error {
	moduleKey, err := domain.NewModuleKey(rawModuleKey)
	if err != nil {
		return err
	}
	if targetUserID == uuid.Nil() || actorID == uuid.Nil() {
		return domain.ErrInvalidUserID
	}
	return s.unit.ExecuteWithRepositoryProvider(
		ctx,
		func(provider *mysql.RepositoryProvider) error {
			if err := requireSuperuser(
				provider.SystemRole(ctx),
				actorID,
			); err != nil {
				return err
			}
			grant, err := provider.ModuleGrant(ctx).
				FindByUserIDAndModuleKey(targetUserID, moduleKey)
			if err != nil || grant != nil {
				return err
			}
			grant, err = domain.NewModuleGrant(
				targetUserID,
				moduleKey,
				&actorID,
				s.now(),
			)
			if err != nil {
				return err
			}
			err = provider.ModuleGrant(ctx).Store(grant)
			if errors.Is(err, domain.ErrModuleGrantAlreadyExists) {
				return nil
			}
			return err
		},
	)
}

func (s *Service) RevokeAccess(
	ctx context.Context,
	actorID, targetUserID uuid.UUID,
	rawModuleKey string,
) error {
	moduleKey, err := domain.NewModuleKey(rawModuleKey)
	if err != nil {
		return err
	}
	if targetUserID == uuid.Nil() || actorID == uuid.Nil() {
		return domain.ErrInvalidUserID
	}
	return s.unit.ExecuteWithRepositoryProvider(
		ctx,
		func(provider *mysql.RepositoryProvider) error {
			if err := requireSuperuser(
				provider.SystemRole(ctx),
				actorID,
			); err != nil {
				return err
			}
			return provider.ModuleGrant(ctx).
				Delete(targetUserID, moduleKey)
		},
	)
}

func (s *Service) CheckAccess(
	ctx context.Context,
	userID uuid.UUID,
	rawModuleKey string,
) (bool, error) {
	moduleKey, err := domain.NewModuleKey(rawModuleKey)
	if err != nil {
		return false, err
	}
	if userID == uuid.Nil() {
		return false, domain.ErrInvalidUserID
	}
	var granted bool
	err = s.unit.ExecuteWithRepositoryProvider(
		ctx,
		func(provider *mysql.RepositoryProvider) error {
			superuser, err := isSuperuser(provider.SystemRole(ctx), userID)
			if err != nil || superuser {
				granted = superuser
				return err
			}
			grant, err := provider.ModuleGrant(ctx).
				FindByUserIDAndModuleKey(userID, moduleKey)
			granted = grant != nil
			return err
		},
	)
	return granted, err
}

func (s *Service) ListAccess(
	ctx context.Context,
	userID uuid.UUID,
) ([]domain.ModuleGrantData, bool, error) {
	if userID == uuid.Nil() {
		return nil, false, domain.ErrInvalidUserID
	}
	var grants []domain.ModuleGrantData
	var superuser bool
	err := s.unit.ExecuteWithRepositoryProvider(
		ctx,
		func(provider *mysql.RepositoryProvider) error {
			var err error
			superuser, err = isSuperuser(provider.SystemRole(ctx), userID)
			if err != nil {
				return err
			}
			stored, err := provider.ModuleGrant(ctx).
				FindAllByUserID(userID)
			if err != nil {
				return err
			}
			grants = make([]domain.ModuleGrantData, len(stored))
			for i, grant := range stored {
				grants[i] = grant.Data()
			}
			return nil
		},
	)
	return grants, superuser, err
}

func (s *Service) IsSuperuser(
	ctx context.Context,
	userID uuid.UUID,
) (bool, error) {
	if userID == uuid.Nil() {
		return false, domain.ErrInvalidUserID
	}
	var superuser bool
	err := s.unit.ExecuteWithRepositoryProvider(
		ctx,
		func(provider *mysql.RepositoryProvider) error {
			var err error
			superuser, err = isSuperuser(provider.SystemRole(ctx), userID)
			return err
		},
	)
	return superuser, err
}

func (s *Service) BootstrapSuperuser(
	ctx context.Context,
	userID uuid.UUID,
) error {
	assignment, err := domain.NewSystemRoleAssignment(
		userID,
		domain.SystemRoleSuperuser,
	)
	if err != nil {
		return err
	}
	return s.unit.ExecuteWithRepositoryProvider(
		ctx,
		func(provider *mysql.RepositoryProvider) error {
			return provider.InitialSuperuser(ctx).Bootstrap(assignment)
		},
	)
}

func requireSuperuser(
	repository domain.SystemRoleRepository,
	userID uuid.UUID,
) error {
	superuser, err := isSuperuser(repository, userID)
	if err != nil {
		return fmt.Errorf("check superuser: %w", err)
	}
	if !superuser {
		return ErrPermissionDenied
	}
	return nil
}

func isSuperuser(
	repository domain.SystemRoleRepository,
	userID uuid.UUID,
) (bool, error) {
	assignment, err := repository.FindByUserID(userID)
	if err != nil || assignment == nil {
		return false, err
	}
	return assignment.Data().Role.HasGlobalPrivileges(), nil
}

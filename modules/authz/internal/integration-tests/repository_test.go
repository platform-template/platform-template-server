package integration_tests

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	"github.com/stretchr/testify/require"

	"authz/internal/authz/domain"
	authzmysql "authz/internal/authz/infra/mysql"
	"authz/internal/authz/infra/mysql/repository"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

func TestModuleGrantRepositoryStoreFindListAndDelete(t *testing.T) {
	clearTables(t)
	ctx := t.Context()
	repo := repository.NewModuleGrantRepository(ctx, testClient)
	userID, issuerID := uuid.New(), uuid.New()
	inventory := mustModuleKey(t, "inventory")
	shopping := mustModuleKey(t, "shopping")
	grantedAt := time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
	grant := mustGrant(t, userID, inventory, &issuerID, grantedAt)
	require.NoError(t, repo.Store(grant))
	require.NoError(t, repo.Store(mustGrant(t, userID, shopping, nil, grantedAt)))

	found, err := repo.FindByUserIDAndModuleKey(userID, inventory)
	require.NoError(t, err)
	require.Equal(t, grant.Data(), found.Data())

	grants, err := repo.FindAllByUserID(userID)
	require.NoError(t, err)
	require.Len(t, grants, 2)
	require.Equal(t, inventory, grants[0].Data().ModuleKey)
	require.Equal(t, shopping, grants[1].Data().ModuleKey)

	require.NoError(t, repo.Delete(userID, inventory))
	found, err = repo.FindByUserIDAndModuleKey(userID, inventory)
	require.NoError(t, err)
	require.Nil(t, found)
	require.NoError(t, repo.Delete(userID, inventory))
}

func TestModuleGrantRepositoryEnforcesUniquePair(t *testing.T) {
	clearTables(t)
	repo := repository.NewModuleGrantRepository(t.Context(), testClient)
	grant := mustGrant(t, uuid.New(), mustModuleKey(t, "inventory"), nil, time.Now())
	require.NoError(t, repo.Store(grant))
	require.ErrorIs(t, repo.Store(grant), domain.ErrModuleGrantAlreadyExists)
}

func TestSystemRoleRepositoryStoreFindAndDefaultRoleAbsence(t *testing.T) {
	clearTables(t)
	repo := repository.NewSystemRoleRepository(t.Context(), testClient)
	userID := uuid.New()

	assignment, err := repo.FindByUserID(userID)
	require.NoError(t, err)
	require.Nil(t, assignment)

	superuser, err := domain.NewSystemRoleAssignment(userID, domain.SystemRoleSuperuser)
	require.NoError(t, err)
	require.NoError(t, repo.Store(superuser))
	require.NoError(t, repo.Store(superuser))

	assignment, err = repo.FindByUserID(userID)
	require.NoError(t, err)
	require.Equal(t, superuser.Data(), assignment.Data())
	var count int
	require.NoError(t, testClient.QueryRowContext(t.Context(), "test.authz.role_count", "SELECT COUNT(*) FROM system_roles WHERE user_id = ?", userID.String()).Scan(&count))
	require.Equal(t, 1, count)
}

func TestUnitOfWorkRollsBackModuleGrant(t *testing.T) {
	clearTables(t)
	userID := uuid.New()
	moduleKey := mustModuleKey(t, "inventory")
	unit := uow.NewUnitOfWork(mysql.NewConnectionPool(testClient), authzmysql.NewRepositoryProvider)
	expected := errors.New("abort transaction")
	err := unit.ExecuteWithRepositoryProvider(context.Background(), func(provider *authzmysql.RepositoryProvider) error {
		if err := provider.ModuleGrant(context.Background()).Store(mustGrant(t, userID, moduleKey, nil, time.Now())); err != nil {
			return err
		}
		return expected
	})
	require.ErrorIs(t, err, expected)

	found, err := repository.NewModuleGrantRepository(t.Context(), testClient).FindByUserIDAndModuleKey(userID, moduleKey)
	require.NoError(t, err)
	require.Nil(t, found)
}

func TestModuleGrantRepositoryConcurrentStoreKeepsOneRecord(t *testing.T) {
	clearTables(t)
	grant := mustGrant(t, uuid.New(), mustModuleKey(t, "notifications"), nil, time.Now())
	const workers = 8
	errs := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			errs <- repository.NewModuleGrantRepository(context.Background(), testClient).Store(grant)
		}()
	}
	group.Wait()
	close(errs)

	successes := 0
	for err := range errs {
		if err == nil {
			successes++
			continue
		}
		require.ErrorIs(t, err, domain.ErrModuleGrantAlreadyExists)
	}
	require.Equal(t, 1, successes)

	grants, err := repository.NewModuleGrantRepository(t.Context(), testClient).FindAllByUserID(grant.Data().UserID)
	require.NoError(t, err)
	require.Len(t, grants, 1)
}

func mustModuleKey(t *testing.T, value string) domain.ModuleKey {
	t.Helper()
	key, err := domain.NewModuleKey(value)
	require.NoError(t, err)
	return key
}

func mustGrant(t *testing.T, userID uuid.UUID, key domain.ModuleKey, issuer *uuid.UUID, grantedAt time.Time) *domain.ModuleGrant {
	t.Helper()
	grant, err := domain.NewModuleGrant(userID, key, issuer, grantedAt)
	require.NoError(t, err)
	return grant
}

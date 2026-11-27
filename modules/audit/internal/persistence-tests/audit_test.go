package persistence_tests

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/event/inbox"
	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"

	"audit/internal/audit/domain"
	auditmysql "audit/internal/audit/infra/mysql"
	"audit/internal/audit/infra/mysql/migrations"
	"audit/internal/audit/infra/mysql/repository"
)

var testClient mysql.TransactionalClient

const (
	authzModule   = "authz"
	accessGranted = "access.granted"
	moduleTarget  = "module"
)

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) (exitCode int) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	container, err := tcmysql.Run(
		ctx,
		"mysql:8.4",
		tcmysql.WithDatabase("audit_test"),
		tcmysql.WithUsername("audit"),
		tcmysql.WithPassword("audit-test"),
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() {
		if err := container.Terminate(context.Background()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			exitCode = 1
		}
	}()
	dsn, err := container.ConnectionString(ctx, "parseTime=true")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	config, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	config.ParseTime = true
	config.Loc = time.UTC
	connector := mysql.NewConnector()
	if err := connector.Open(config.FormatDSN(), mysql.Config{MaxConnections: 16}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer func() {
		if err := connector.Close(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			exitCode = 1
		}
	}()
	testClient = connector.TransactionalClient()
	runner, err := migrator.New(testClient, slog.Default())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := runner.Migrate(ctx, "audit", migrations.Database(testClient)...); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := runner.Migrate(ctx, "event-inbox", inbox.Migrations(testClient)...); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return m.Run()
}

func makeEntry(t *testing.T, data domain.AuditEntryData) *domain.AuditEntry {
	t.Helper()
	entry, err := domain.NewAuditEntry(data)
	require.NoError(t, err)
	return entry
}

func TestAppendAndGet(t *testing.T) {
	repo := repository.NewAuditRepository(t.Context(), testClient)
	actor := uuid.New()
	data := domain.AuditEntryData{
		EventID:       "append:" + uuid.New().String(),
		ActorID:       &actor,
		ModuleKey:     authzModule,
		Action:        accessGranted,
		TargetType:    moduleTarget,
		TargetID:      "inventory",
		CorrelationID: "corr:123",
		Metadata:      []byte(`{"module":"inventory"}`),
		OccurredAt:    time.Now(),
	}
	entry := makeEntry(t, data)
	err := repo.Append(entry)
	require.NoError(t, err)

	found, err := repo.GetByEventID(data.EventID)
	require.NoError(t, err)
	expected := entry.Data()
	expected.OccurredAt = expected.OccurredAt.Truncate(time.Microsecond)
	expected.RecordedAt = expected.RecordedAt.Truncate(time.Microsecond)
	require.Equal(t, expected, found.Data())

	duplicateData := data
	duplicateData.Action = "access.revoked"
	duplicate := makeEntry(t, duplicateData)
	err = repo.Append(duplicate)
	require.ErrorIs(t, err, domain.ErrAuditEntryDuplicate)
	found, err = repo.GetByEventID(data.EventID)
	require.NoError(t, err)
	require.Equal(t, expected, found.Data())

	found, err = repo.GetByEventID("missing:" + uuid.New().String())
	require.NoError(t, err)
	require.Nil(t, found)
}

func TestStorageConstraints(t *testing.T) {
	repo := repository.NewAuditRepository(t.Context(), testClient)
	valid := domain.AuditEntryData{
		EventID:    uuid.New().String(),
		ModuleKey:  authzModule,
		Action:     accessGranted,
		OccurredAt: time.Now(),
		RecordedAt: time.Now(),
	}
	tests := []struct {
		name   string
		change func(*domain.AuditEntryData)
	}{
		{
			name: "event id too long",
			change: func(data *domain.AuditEntryData) {
				data.EventID = strings.Repeat("a", 256)
			},
		},
		{
			name: "module too long",
			change: func(data *domain.AuditEntryData) {
				data.ModuleKey = strings.Repeat("a", 129)
			},
		},
		{
			name: "action too long",
			change: func(data *domain.AuditEntryData) {
				data.Action = strings.Repeat("a", 129)
			},
		},
		{
			name: "target type too long",
			change: func(data *domain.AuditEntryData) {
				data.TargetType = strings.Repeat("a", 129)
			},
		},
		{
			name: "target id too long",
			change: func(data *domain.AuditEntryData) {
				data.TargetID = strings.Repeat("a", 256)
			},
		},
		{
			name: "correlation too long",
			change: func(data *domain.AuditEntryData) {
				data.CorrelationID = strings.Repeat("a", 256)
			},
		},
		{
			name: "invalid string encoding",
			change: func(data *domain.AuditEntryData) {
				data.EventID = string([]byte{0xff})
			},
		},
		{
			name: "metadata too large",
			change: func(data *domain.AuditEntryData) {
				data.Metadata = []byte(`"` + strings.Repeat("a", 16383) + `"`)
			},
		},
		{
			name: "invalid metadata encoding",
			change: func(data *domain.AuditEntryData) {
				data.Metadata = []byte{'"', 0xff, '"'}
			},
		},
		{
			name: "occurred before mysql range",
			change: func(data *domain.AuditEntryData) {
				data.OccurredAt = time.Date(999, 1, 1, 0, 0, 0, 0, time.UTC)
			},
		},
		{
			name: "recorded after mysql range",
			change: func(data *domain.AuditEntryData) {
				data.RecordedAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			data := valid
			test.change(&data)
			entry, err := domain.RestoreAuditEntry(data)
			require.NoError(t, err)
			err = repo.Append(entry)
			require.ErrorIs(t, err, repository.ErrAuditStorageConstraint)
		})
	}
}

func TestStorageInfrastructureError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	repo := repository.NewAuditRepository(ctx, testClient)
	entry := makeEntry(t, domain.AuditEntryData{
		EventID:    uuid.New().String(),
		ModuleKey:  authzModule,
		Action:     accessGranted,
		OccurredAt: time.Now(),
	})

	err := repo.Append(entry)
	require.ErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, repository.ErrAuditStorageConstraint)
	require.NotErrorIs(t, err, domain.ErrAuditEntryDuplicate)
}

func TestListFiltersAndPagination(t *testing.T) {
	repo := repository.NewAuditRepository(t.Context(), testClient)
	actor, other := uuid.New(), uuid.New()
	base := time.Date(2026, 5, 1, 12, 0, 0, 123456000, time.UTC)
	prefix := uuid.New().String()
	entries := []domain.AuditEntryData{
		{
			EventID:    prefix + "-a",
			ActorID:    &actor,
			ModuleKey:  authzModule,
			Action:     accessGranted,
			TargetType: moduleTarget,
			TargetID:   "inventory",
			OccurredAt: base,
		},
		{
			EventID:    prefix + "-b",
			ActorID:    &actor,
			ModuleKey:  authzModule,
			Action:     "access.revoked",
			TargetType: moduleTarget,
			TargetID:   "shopping",
			OccurredAt: base,
		},
		{
			EventID:    prefix + "-c",
			ActorID:    &other,
			ModuleKey:  "identity",
			Action:     "user.created",
			TargetType: "user",
			TargetID:   prefix,
			OccurredAt: base.Add(time.Second),
		},
		{
			EventID:    prefix + "-d",
			ModuleKey:  "system",
			Action:     "system.started",
			OccurredAt: base.Add(2 * time.Second),
		},
	}
	for _, data := range entries {
		entry := makeEntry(t, data)
		err := repo.Append(entry)
		require.NoError(t, err)
	}
	from, to := base, base.Add(2*time.Second)
	tests := []struct {
		name     string
		filter   domain.AuditFilter
		expected []string
	}{
		{
			name:     "actor",
			filter:   domain.AuditFilter{ActorID: &actor, Limit: 10},
			expected: []string{prefix + "-b", prefix + "-a"},
		},
		{
			name: "module",
			filter: domain.AuditFilter{
				ActorID:   &other,
				ModuleKey: "identity",
				Limit:     10,
			},
			expected: []string{prefix + "-c"},
		},
		{
			name: "action",
			filter: domain.AuditFilter{
				ActorID: &actor,
				Action:  accessGranted,
				Limit:   10,
			},
			expected: []string{prefix + "-a"},
		},
		{
			name: "target",
			filter: domain.AuditFilter{
				TargetType: "user",
				TargetID:   prefix,
				Limit:      10,
			},
			expected: []string{prefix + "-c"},
		},
		{
			name: "time",
			filter: domain.AuditFilter{
				ActorID: &actor,
				From:    &from,
				To:      &to,
				Limit:   10,
			},
			expected: []string{prefix + "-b", prefix + "-a"},
		},
		{
			name: "time range without other filters",
			filter: domain.AuditFilter{
				From:  &from,
				To:    &to,
				Limit: 10,
			},
			expected: []string{prefix + "-c", prefix + "-b", prefix + "-a"},
		},
		{
			name: "page",
			filter: domain.AuditFilter{
				ActorID: &actor,
				Limit:   1,
				Offset:  1,
			},
			expected: []string{prefix + "-a"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			found, err := repo.List(test.filter)
			require.NoError(t, err)
			ids := make([]string, 0, len(found))
			for _, entry := range found {
				ids = append(ids, entry.Data().EventID)
			}
			require.Equal(t, test.expected, ids)
		})
	}
	_, err := repo.List(domain.AuditFilter{Limit: 101})
	require.ErrorIs(t, err, domain.ErrInvalidAuditFilter)
	system, err := repo.GetByEventID(prefix + "-d")
	require.NoError(t, err)
	require.Nil(t, system.Data().ActorID)
}

func TestConcurrentAppend(t *testing.T) {
	entry := makeEntry(
		t,
		domain.AuditEntryData{
			EventID:    uuid.New().String(),
			ModuleKey:  "auth",
			Action:     "login",
			OccurredAt: time.Now(),
		},
	)
	const workers = 8
	errorsChannel := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			repo := repository.NewAuditRepository(context.Background(), testClient)
			errorsChannel <- repo.Append(entry)
		}()
	}
	group.Wait()
	close(errorsChannel)
	successes := 0
	for err := range errorsChannel {
		if err == nil {
			successes++
			continue
		}
		require.ErrorIs(t, err, domain.ErrAuditEntryDuplicate)
	}
	require.Equal(t, 1, successes)
}

func TestUnitOfWorkRollback(t *testing.T) {
	entry := makeEntry(
		t,
		domain.AuditEntryData{
			EventID:    uuid.New().String(),
			ModuleKey:  "auth",
			Action:     "login",
			OccurredAt: time.Now(),
		},
	)
	unit := uow.NewUnitOfWork(mysql.NewConnectionPool(testClient), auditmysql.NewRepositoryProvider)
	expected := errors.New("rollback")
	err := unit.ExecuteWithRepositoryProvider(t.Context(), func(provider *auditmysql.RepositoryProvider) error {
		if err := provider.Audit(t.Context()).Append(entry); err != nil {
			return err
		}
		return expected
	})
	require.ErrorIs(t, err, expected)
	found, err := repository.NewAuditRepository(t.Context(), testClient).GetByEventID(entry.Data().EventID)
	require.NoError(t, err)
	require.Nil(t, found)
}

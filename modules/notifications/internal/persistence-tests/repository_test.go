package integration_tests

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"
	"uuid"

	"notifications/internal/notifications/domain"
	nmysql "notifications/internal/notifications/infra/mysql"
	"notifications/internal/notifications/infra/mysql/migrations"
	"notifications/internal/notifications/infra/mysql/repository"

	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	driver "github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/require"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
)

var client mysql.TransactionalClient

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	container, err := tcmysql.Run(
		ctx,
		"mysql:8.4",
		tcmysql.WithDatabase("notifications_test"),
		tcmysql.WithUsername("notifications"),
		tcmysql.WithPassword("integration-password"),
	)
	if err != nil {
		cancel()
		os.Exit(1)
	}
	dsn, err := container.ConnectionString(ctx, "parseTime=true")
	if err != nil {
		_ = container.Terminate(context.Background())
		cancel()
		os.Exit(1)
	}
	config, err := driver.ParseDSN(dsn)
	if err != nil {
		_ = container.Terminate(context.Background())
		cancel()
		os.Exit(1)
	}
	config.ParseTime = true
	config.Loc = time.UTC
	connector := mysql.NewConnector()
	if err := connector.Open(config.FormatDSN(), mysql.Config{MaxConnections: 16}); err != nil {
		_ = container.Terminate(context.Background())
		cancel()
		os.Exit(1)
	}
	client = connector.TransactionalClient()
	runner, err := migrator.New(client, slog.Default())
	if err != nil {
		_ = connector.Close()
		_ = container.Terminate(context.Background())
		cancel()
		os.Exit(1)
	}
	if err := runner.Migrate(ctx, "notifications", migrations.Database(client)...); err != nil {
		_ = connector.Close()
		_ = container.Terminate(context.Background())
		cancel()
		os.Exit(1)
	}
	cancel()
	code := m.Run()
	_ = connector.Close()
	_ = container.Terminate(context.Background())
	os.Exit(code)
}

func makeNotification(t *testing.T, user uuid.UUID, created time.Time, key *string) *domain.Notification {
	t.Helper()
	n, err := domain.NewNotification(
		domain.NotificationData{
			ID:             uuid.New(),
			UserID:         user,
			Kind:           "access.granted",
			Title:          "Test",
			CreatedAt:      created,
			IdempotencyKey: key,
		},
	)
	require.NoError(t, err)
	return n
}

func TestRepository(t *testing.T) {
	ctx := t.Context()
	repo := repository.New(ctx, client)
	user, other := uuid.New(), uuid.New()
	created := time.Now().UTC().Truncate(time.Microsecond)
	first := makeNotification(t, user, created, nil)
	second := makeNotification(t, user, created.Add(time.Second), nil)
	third := makeNotification(t, user, created.Add(time.Second), nil)
	foreign := makeNotification(t, other, created, nil)
	for _, n := range []*domain.Notification{first, second, third, foreign} {
		err := repo.Create(n)
		require.NoError(t, err)
	}

	found, err := repo.GetByID(user, first.Data().ID)
	require.NoError(t, err)
	require.Equal(t, first.Data(), found.Data())

	found, err = repo.GetByID(other, first.Data().ID)
	require.NoError(t, err)
	require.Nil(t, found)

	found, err = repo.GetByID(user, uuid.New())
	require.NoError(t, err)
	require.Nil(t, found)

	all, err := repo.ListByUser(user, domain.ListOptions{Limit: 10})
	require.NoError(t, err)
	require.Len(t, all, 3)
	require.Equal(t, first.Data().ID, all[2].Data().ID)
	require.Greater(t, all[0].Data().ID.String(), all[1].Data().ID.String())

	page, err := repo.ListByUser(user, domain.ListOptions{Limit: 1, Offset: 1})
	require.NoError(t, err)
	require.Equal(t, all[1].Data().ID, page[0].Data().ID)

	count, err := repo.CountUnread(user)
	require.NoError(t, err)
	require.EqualValues(t, 3, count)

	err = repo.MarkAsRead(other, first.Data().ID, created)
	require.ErrorIs(t, err, domain.ErrNotFound)

	err = repo.MarkAsRead(user, first.Data().ID, created.Add(-time.Second))
	require.ErrorIs(t, err, domain.ErrInvalidReadAt)

	read := created.Add(time.Minute)
	err = repo.MarkAsRead(user, first.Data().ID, read)
	require.NoError(t, err)

	err = repo.MarkAsRead(user, first.Data().ID, read.Add(time.Hour))
	require.NoError(t, err)

	found, err = repo.GetByID(user, first.Data().ID)
	require.NoError(t, err)
	require.Equal(t, read, *found.Data().ReadAt)

	unread, err := repo.ListByUser(user, domain.ListOptions{Limit: 10, UnreadOnly: true})
	require.NoError(t, err)
	require.Len(t, unread, 2)

	count, err = repo.CountUnread(user)
	require.NoError(t, err)
	require.EqualValues(t, 2, count)
}

func TestIdempotencyAndConcurrentRead(t *testing.T) {
	ctx := t.Context()
	repo := repository.New(ctx, client)
	user := uuid.New()
	now := time.Now().UTC().Truncate(time.Microsecond)
	key, otherKey := "event:1", "event:2"
	first := makeNotification(t, user, now, &key)
	err := repo.Create(first)
	require.NoError(t, err)

	duplicate := makeNotification(t, user, now, &key)
	err = repo.Create(duplicate)
	require.ErrorIs(t, err, domain.ErrDuplicate)

	otherNotifications := []*domain.Notification{
		makeNotification(t, user, now, &otherKey),
		makeNotification(t, uuid.New(), now, &key),
		makeNotification(t, user, now, nil),
		makeNotification(t, user, now, nil),
	}
	for _, notification := range otherNotifications {
		err = repo.Create(notification)
		require.NoError(t, err)
	}

	concurrentKey := "concurrent"
	const workers = 8
	errs := make(chan error, workers)
	var group sync.WaitGroup
	for range workers {
		n := makeNotification(t, user, now, &concurrentKey)
		group.Add(1)
		go func() {
			defer group.Done()
			concurrentRepo := repository.New(context.Background(), client)
			errs <- concurrentRepo.Create(n)
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
		require.ErrorIs(t, err, domain.ErrDuplicate)
	}
	require.Equal(t, 1, successes)

	reads := make(chan error, workers)
	for i := range workers {
		group.Add(1)
		go func() {
			defer group.Done()
			concurrentRepo := repository.New(context.Background(), client)
			readAt := now.Add(time.Duration(i+1) * time.Second)
			reads <- concurrentRepo.MarkAsRead(user, first.Data().ID, readAt)
		}()
	}
	group.Wait()
	close(reads)

	for err := range reads {
		require.NoError(t, err)
	}

	found, err := repo.GetByID(user, first.Data().ID)
	require.NoError(t, err)
	initial := *found.Data().ReadAt

	err = repo.MarkAsRead(user, first.Data().ID, now.Add(time.Hour))
	require.NoError(t, err)

	found, err = repo.GetByID(user, first.Data().ID)
	require.NoError(t, err)
	require.Equal(t, initial, *found.Data().ReadAt)
}

func TestUnitOfWork(t *testing.T) {
	ctx := t.Context()
	user := uuid.New()
	first := makeNotification(t, user, time.Now(), nil)
	second := makeNotification(t, user, time.Now(), nil)
	unit := uow.NewUnitOfWork(mysql.NewConnectionPool(client), nmysql.NewRepositoryProvider)

	err := unit.ExecuteWithRepositoryProvider(ctx, func(provider *nmysql.RepositoryProvider) error {
		return provider.Notification(ctx).Create(first)
	})
	require.NoError(t, err)

	expected := errors.New("rollback")
	err = unit.ExecuteWithRepositoryProvider(ctx, func(p *nmysql.RepositoryProvider) error {
		if err := p.Notification(ctx).Create(second); err != nil {
			return err
		}
		if err := p.Notification(ctx).MarkAsRead(user, first.Data().ID, time.Now().Add(time.Minute)); err != nil {
			return err
		}
		return expected
	})
	require.ErrorIs(t, err, expected)

	repo := repository.New(ctx, client)
	found, err := repo.GetByID(user, second.Data().ID)
	require.NoError(t, err)
	require.Nil(t, found)

	found, err = repo.GetByID(user, first.Data().ID)
	require.NoError(t, err)
	require.Nil(t, found.Data().ReadAt)
}

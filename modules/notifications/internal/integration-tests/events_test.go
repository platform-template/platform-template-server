package integration_tests

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"
	"uuid"

	notificationsevents "notifications/api/events"
	"notifications/internal/notifications/app"
	handlers "notifications/internal/notifications/infra/handlers/events"
	nmysql "notifications/internal/notifications/infra/mysql"

	"github.com/distributed-programming-2026/go-sdk/pkg/amqp"
	"github.com/distributed-programming-2026/go-sdk/pkg/event"
	eventamqp "github.com/distributed-programming-2026/go-sdk/pkg/event/amqp"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	domainevent "github.com/distributed-programming-2026/lib/event"
	"github.com/stretchr/testify/require"
)

func TestGenericNotificationDelivery(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	connection := amqp.NewConnection(
		moduleID,
		amqp.ConnectionConfig{URL: testEnv.conf.AMQP.URL, ConnectTimeout: time.Second},
		logger,
	)
	defer func() { require.NoError(t, connection.Stop()) }()
	dispatcher, err := eventamqp.NewDispatcher(connection.Producer(domainevent.DomainExchange(), nil, nil))
	require.NoError(t, err)
	require.NoError(t, connection.Start())
	for _, producer := range []string{"first-service", "new-service"} {
		user := uuid.New()
		envelope, err := event.New(
			t.Context(),
			producer,
			"NotificationRequested",
			&notificationsevents.NotificationRequested{
				UserId: user.String(), Kind: "arbitrary.kind", Title: "Hello", Body: "Message",
			},
		)
		require.NoError(t, err)
		require.NoError(t, dispatcher.Dispatch(t.Context(), envelope))
		require.NoError(t, dispatcher.Dispatch(t.Context(), envelope))
		waitHandled(t, envelope, 2)
		var count int
		var kind, title, body string
		client := testEnv.connector.TransactionalClient()
		require.NoError(
			t,
			client.QueryRowContext(t.Context(), "test.notification.count", `SELECT COUNT(*) FROM notifications WHERE user_id = ?`, user[:]).
				Scan(&count),
		)
		require.Equal(t, 1, count)
		require.NoError(
			t,
			client.QueryRowContext(t.Context(), "test.notification.content", `SELECT kind, title, body FROM notifications WHERE user_id = ?`, user[:]).
				Scan(&kind, &title, &body),
		)
		require.Equal(t, "arbitrary.kind", kind)
		require.Equal(t, "Hello", title)
		require.Equal(t, "Message", body)
	}
}

func TestNotificationInboxAtomicity(t *testing.T) {
	client := testEnv.connector.TransactionalClient()
	unit := uow.NewUnitOfWork(mysql.NewConnectionPool(client), nmysql.NewRepositoryProvider)
	handler := handlers.NewHandler(slog.Default(), app.NewService(unit, nil))
	user := uuid.New()
	envelope, err := event.New(
		t.Context(),
		"independent-service",
		"NotificationRequested",
		&notificationsevents.NotificationRequested{
			UserId: user.String(), Kind: "arbitrary.kind", Title: "Test",
		},
	)
	require.NoError(t, err)
	failure := errors.New("failure after notification insert")
	failing := event.HandlerFunc(func(ctx context.Context, item event.Envelope) error {
		if err := handler.Handle(ctx, item); err != nil {
			return err
		}
		return failure
	})
	consumer := "notification-test-" + uuid.New().String()
	wrapped, err := handlers.WithInbox(failing, unit, consumer)
	require.NoError(t, err)
	require.ErrorIs(t, wrapped.Handle(t.Context(), envelope), failure)
	var count int
	require.NoError(
		t,
		client.QueryRowContext(t.Context(), "test.notification.count", `SELECT COUNT(*) FROM notifications WHERE user_id = ?`, user[:]).
			Scan(&count),
	)
	require.Zero(t, count)
	require.NoError(
		t,
		client.QueryRowContext(t.Context(), "test.inbox.count", `SELECT COUNT(*) FROM event_inbox WHERE consumer = ? AND event_id = ?`, consumer, envelope.ID()).
			Scan(&count),
	)
	require.Zero(t, count)
	wrapped, err = handlers.WithInbox(handler, unit, consumer)
	require.NoError(t, err)
	require.NoError(t, wrapped.Handle(t.Context(), envelope))
	require.NoError(t, wrapped.Handle(t.Context(), envelope))
	require.NoError(
		t,
		client.QueryRowContext(t.Context(), "test.notification.count", `SELECT COUNT(*) FROM notifications WHERE user_id = ?`, user[:]).
			Scan(&count),
	)
	require.Equal(t, 1, count)
}

package persistence_tests

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/event"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	"github.com/stretchr/testify/require"

	"audit/internal/audit/app"
	auditevents "audit/internal/audit/infra/handlers/events"
	auditmysql "audit/internal/audit/infra/mysql"
	inventoryevents "inventory/api/events"
)

func TestInventoryEventInboxAtomicity(t *testing.T) {
	unit := uow.NewUnitOfWork(mysql.NewConnectionPool(testClient), auditmysql.NewRepositoryProvider)
	service := app.NewService(unit, nil)
	business := auditevents.NewHandler(slog.Default(), service)
	actor, inventoryID := uuid.New(), uuid.New()
	envelope, err := event.New(t.Context(), "inventory", "InventoryCreated", &inventoryevents.InventoryCreated{
		InventoryId: inventoryID.String(), Name: "Холодильник", ActorId: actor.String(),
	})
	require.NoError(t, err)
	failure := errors.New("failure after audit insert")
	failing := event.HandlerFunc(func(ctx context.Context, item event.Envelope) error {
		if err := business.Handle(ctx, item); err != nil {
			return err
		}
		return failure
	})
	consumer := "audit-test-" + uuid.New().String()
	handler, err := auditevents.WithInbox(failing, unit, consumer)
	require.NoError(t, err)
	require.ErrorIs(t, handler.Handle(t.Context(), envelope), failure)
	var count int
	require.NoError(
		t,
		testClient.QueryRowContext(t.Context(), "test.audit.count", `SELECT COUNT(*) FROM audit_entries WHERE event_id = ?`, envelope.ID()).
			Scan(&count),
	)
	require.Zero(t, count)
	require.NoError(
		t,
		testClient.QueryRowContext(t.Context(), "test.inbox.count", `SELECT COUNT(*) FROM event_inbox WHERE consumer = ? AND event_id = ?`, consumer, envelope.ID()).
			Scan(&count),
	)
	require.Zero(t, count)

	handler, err = auditevents.WithInbox(business, unit, consumer)
	require.NoError(t, err)
	require.NoError(t, handler.Handle(t.Context(), envelope))
	require.NoError(t, handler.Handle(t.Context(), envelope))
	entry, err := service.GetAuditEntry(t.Context(), envelope.ID())
	require.NoError(t, err)
	data := entry.Data()
	require.Equal(t, "inventory.created", data.Action)
	require.Equal(t, inventoryID.String(), data.TargetID)
	require.Equal(t, actor, *data.ActorID)
	require.Equal(t, envelope.CorrelationID(), data.CorrelationID)
	require.JSONEq(t, `{"name":"Холодильник"}`, string(data.Metadata))
	require.NoError(
		t,
		testClient.QueryRowContext(t.Context(), "test.inbox.count", `SELECT COUNT(*) FROM event_inbox WHERE consumer = ? AND event_id = ?`, consumer, envelope.ID()).
			Scan(&count),
	)
	require.Equal(t, 1, count)
}

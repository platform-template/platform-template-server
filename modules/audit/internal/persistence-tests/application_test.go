package persistence_tests

import (
	"testing"
	"time"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	"github.com/stretchr/testify/require"

	"audit/internal/audit/app"
	"audit/internal/audit/domain"
	auditmysql "audit/internal/audit/infra/mysql"
)

func TestAuditApplication(t *testing.T) {
	unit := uow.NewUnitOfWork(mysql.NewConnectionPool(testClient), auditmysql.NewRepositoryProvider)
	svc := app.NewService(unit, nil)
	actor := uuid.New()
	metadata := []byte(`{"source":"test"}`)
	input := app.RecordAuditEntryInput{
		EventID:    uuid.New().String(),
		ActorID:    &actor,
		ModuleKey:  "application_test",
		Action:     "application.login",
		Metadata:   metadata,
		OccurredAt: time.Now(),
	}
	entry, err := svc.RecordAuditEntry(t.Context(), input)
	require.NoError(t, err)
	require.False(t, entry.Data().RecordedAt.IsZero())
	metadata[0] = 'x'
	require.JSONEq(t, `{"source":"test"}`, string(entry.Data().Metadata))
	input.Metadata = []byte(`{}`)
	_, err = svc.RecordAuditEntry(t.Context(), input)
	require.ErrorIs(t, err, domain.ErrAuditEntryDuplicate)

	found, err := svc.GetAuditEntry(t.Context(), input.EventID)
	require.NoError(t, err)
	require.Equal(t, input.EventID, found.Data().EventID)
	require.JSONEq(t, `{"source":"test"}`, string(found.Data().Metadata))
	_, err = svc.GetAuditEntry(t.Context(), "missing:"+uuid.New().String())
	require.ErrorIs(t, err, domain.ErrAuditEntryNotFound)

	entries, err := svc.ListAuditEntries(t.Context(), domain.AuditFilter{ActorID: &actor})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	emptyActor := uuid.New()
	entries, err = svc.ListAuditEntries(t.Context(), domain.AuditFilter{ActorID: &emptyActor})
	require.NoError(t, err)
	require.NotNil(t, entries)
	require.Empty(t, entries)

	for range 20 {
		input.EventID = uuid.New().String()
		_, err = svc.RecordAuditEntry(t.Context(), input)
		require.NoError(t, err)
	}
	entries, err = svc.ListAuditEntries(t.Context(), domain.AuditFilter{ActorID: &actor})
	require.NoError(t, err)
	require.Len(t, entries, 20)
	entries, err = svc.ListAuditEntries(t.Context(), domain.AuditFilter{
		ActorID: &actor,
		Limit:   20,
		Offset:  20,
	})
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

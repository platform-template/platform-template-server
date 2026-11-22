package app

import (
	"context"
	"errors"
	"testing"
	"time"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	"github.com/stretchr/testify/require"

	"audit/internal/audit/domain"
	"audit/internal/audit/infra/mysql"
)

type failingUnit struct {
	uow.UnitOfWorkWithRepositoryProvider[*mysql.RepositoryProvider]
	err error
}

func (u failingUnit) ExecuteWithRepositoryProvider(_ context.Context, _ func(*mysql.RepositoryProvider) error) error {
	return u.err
}

func TestAuditValidationAndUnitErrors(t *testing.T) {
	expected := errors.New("unit failed")
	svc := NewService(failingUnit{err: expected}, nil)
	input := RecordAuditEntryInput{
		EventID:    "event",
		ModuleKey:  "auth",
		Action:     "login",
		OccurredAt: time.Now(),
	}
	_, err := svc.RecordAuditEntry(t.Context(), input)
	require.ErrorIs(t, err, expected)
	input.EventID = " "
	_, err = svc.RecordAuditEntry(t.Context(), input)
	require.ErrorIs(t, err, domain.ErrInvalidAuditEntry)

	_, err = svc.GetAuditEntry(t.Context(), "event")
	require.ErrorIs(t, err, expected)
	_, err = svc.ListAuditEntries(t.Context(), domain.AuditFilter{})
	require.ErrorIs(t, err, expected)

	zero := uuid.Nil()
	invalidTime := time.Time{}
	for _, filter := range []domain.AuditFilter{
		{Limit: -1},
		{Limit: 101},
		{Offset: -1},
		{ActorID: &zero},
		{From: &invalidTime},
		{To: &invalidTime},
	} {
		_, err := svc.ListAuditEntries(t.Context(), filter)
		require.ErrorIs(t, err, domain.ErrInvalidAuditFilter)
	}

	duplicate := NewService(failingUnit{err: domain.ErrAuditEntryDuplicate}, nil)
	input.EventID = "event"
	_, err = duplicate.RecordAuditEntry(t.Context(), input)
	require.ErrorIs(t, err, domain.ErrAuditEntryDuplicate)
}

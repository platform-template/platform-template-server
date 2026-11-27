package persistence_tests

import (
	"context"
	"errors"
	"testing"
	"uuid"

	auditevents "audit/api/events"
	"mealplanner/internal/mealplanner/app"
	"mealplanner/internal/mealplanner/domain"
	eventfactory "mealplanner/internal/mealplanner/infra/event"
	mealdb "mealplanner/internal/mealplanner/infra/mysql"

	"github.com/distributed-programming-2026/go-sdk/pkg/event"
	"github.com/distributed-programming-2026/go-sdk/pkg/event/outbox"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	"github.com/stretchr/testify/require"
)

func testPlanApplication(
	ctx context.Context,
	t *testing.T,
	client mysql.TransactionalClient,
	unit uow.UnitOfWorkWithRepositoryProvider[*mealdb.RepositoryProvider],
) {
	t.Helper()
	transport := event.DispatcherFunc(func(context.Context, event.Envelope) error { return nil })
	dispatcher, _, err := outbox.Decorate(transport, uow.NewLockableUnitOfWork(unit), outbox.Config{Transport: "test"})
	require.NoError(t, err)
	service := app.NewService(unit, eventfactory.NewDispatcherFactory("mealplanner", dispatcher))
	owner, stranger := uuid.New(), uuid.New()
	plan, err := service.CreatePlan(ctx, owner, "Plan")
	require.NoError(t, err)
	_, err = service.GetPlan(ctx, stranger, plan.ID())
	require.ErrorIs(t, err, domain.ErrPlanNotFound)
	require.ErrorIs(t, service.RenamePlan(ctx, stranger, plan.ID(), "No"), domain.ErrPlanNotFound)
	require.NoError(t, service.RenamePlan(ctx, owner, plan.ID(), "New name"))
	require.NoError(t, service.RenamePlan(ctx, owner, plan.ID(), "New name"))
	plans, err := service.ListPlans(ctx, owner)
	require.NoError(t, err)
	require.Len(t, plans, 1)
	rows, err := client.QueryContext(ctx, "test.outbox", `SELECT payload FROM event_outbox ORDER BY id`)
	require.NoError(t, err)
	var actions []string
	for rows.Next() {
		var raw []byte
		require.NoError(t, rows.Scan(&raw))
		envelope, err := event.Unmarshal(raw)
		require.NoError(t, err)
		require.Equal(t, "AuditRecordRequested", envelope.Type())
		var record auditevents.AuditRecordRequested
		require.NoError(t, envelope.UnmarshalPayload(&record))
		require.Equal(t, owner.String(), record.ActorId)
		require.Equal(t, plan.ID().String(), record.TargetId)
		actions = append(actions, record.Action)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.Equal(t, []string{"meal_plan.created", "meal_plan.renamed"}, actions)
	rollback := errors.New("force rollback")
	operationCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	var rolledID uuid.UUID
	err = unit.ExecuteWithRepositoryProvider(operationCtx, func(_ *mealdb.RepositoryProvider) error {
		created, err := service.CreatePlan(operationCtx, owner, "Rolled back")
		if err != nil {
			return err
		}
		rolledID = created.ID()
		return rollback
	})
	require.ErrorIs(t, err, rollback)
	_, err = service.GetPlan(ctx, owner, rolledID)
	require.ErrorIs(t, err, domain.ErrPlanNotFound)
	var count int
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.outbox.count", `SELECT COUNT(*) FROM event_outbox`).Scan(&count),
	)
	require.Equal(t, 2, count)
	failing := app.NewService(unit, nil)
	_, err = failing.CreatePlan(ctx, owner, "No dispatcher")
	require.Error(t, err)
	plans, err = service.ListPlans(ctx, owner)
	require.NoError(t, err)
	require.Len(t, plans, 1)
}

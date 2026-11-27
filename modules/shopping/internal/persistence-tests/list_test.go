package persistence_tests

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"testing"
	"time"
	"uuid"

	auditevents "audit/api/events"
	shoppingapi "shopping/api/server/internalapi"
	"shopping/internal/shopping/app"
	"shopping/internal/shopping/domain"
	eventfactory "shopping/internal/shopping/infra/event"
	handler "shopping/internal/shopping/infra/handlers/internalapi"
	shoppingdb "shopping/internal/shopping/infra/mysql"
	"shopping/internal/shopping/infra/mysql/migrations"

	"github.com/distributed-programming-2026/go-sdk/pkg/event"
	"github.com/distributed-programming-2026/go-sdk/pkg/event/inbox"
	"github.com/distributed-programming-2026/go-sdk/pkg/event/outbox"
	"github.com/distributed-programming-2026/go-sdk/pkg/migrator"
	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	"github.com/stretchr/testify/require"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

func TestShoppingLists(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	container, err := tcmysql.Run(
		ctx,
		"mysql:8.4",
		tcmysql.WithDatabase("shopping_test"),
		tcmysql.WithUsername("shopping"),
		tcmysql.WithPassword("test-password"),
	)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, container.Terminate(context.Background())) })
	dsn, err := container.ConnectionString(ctx, "parseTime=true")
	require.NoError(t, err)
	connector := mysql.NewConnector()
	require.NoError(t, connector.Open(dsn, mysql.Config{MaxConnections: 10}))
	t.Cleanup(func() { require.NoError(t, connector.Close()) })
	client := connector.TransactionalClient()
	runner, err := migrator.New(client, slog.Default())
	require.NoError(t, err)
	require.NoError(t, runner.Migrate(ctx, "shopping", migrations.Database(client)...))
	require.NoError(t, runner.Migrate(ctx, "event-outbox", outbox.Migrations(client)...))
	require.NoError(t, runner.Migrate(ctx, "event-inbox", inbox.Migrations(client)...))
	unit := uow.NewUnitOfWork(mysql.NewConnectionPool(client), shoppingdb.NewRepositoryProvider)
	dispatcher, _, err := outbox.Decorate(
		event.DispatcherFunc(func(context.Context, event.Envelope) error { return nil }),
		uow.NewLockableUnitOfWork(unit),
		outbox.Config{Transport: "test"},
	)
	require.NoError(t, err)
	service := app.NewService(unit, eventfactory.NewDispatcherFactory("shopping", dispatcher))
	owner, stranger := uuid.New(), uuid.New()
	list, err := service.CreateList(ctx, owner, "Week")
	require.NoError(t, err)
	require.NoError(t, service.RenameList(ctx, owner, list.ID(), "New"))
	require.NoError(t, service.RenameList(ctx, owner, list.ID(), "New"))
	_, err = service.GetList(ctx, stranger, list.ID())
	require.ErrorIs(t, err, domain.ErrListNotFound)
	require.ErrorIs(t, service.RenameList(ctx, stranger, list.ID(), "No"), domain.ErrListNotFound)
	lists, err := service.ListLists(ctx, owner)
	require.NoError(t, err)
	require.Len(t, lists, 1)
	require.Equal(t, "New", lists[0].Name())
	rows, err := client.QueryContext(ctx, "test.audit", `SELECT payload FROM event_outbox ORDER BY id`)
	require.NoError(t, err)
	var actions []string
	for rows.Next() {
		var raw []byte
		require.NoError(t, rows.Scan(&raw))
		envelope, err := event.Unmarshal(raw)
		require.NoError(t, err)
		var record auditevents.AuditRecordRequested
		require.Equal(t, "AuditRecordRequested", envelope.Type())
		require.NoError(t, envelope.UnmarshalPayload(&record))
		require.Equal(t, owner.String(), record.ActorId)
		actions = append(actions, record.Action)
	}
	require.NoError(t, rows.Err())
	require.NoError(t, rows.Close())
	require.Equal(t, []string{"shopping_list.created", "shopping_list.renamed"}, actions)
	failure := errors.New("rollback")
	operationCtx, done := context.WithCancel(ctx)
	defer done()
	var rollbackID uuid.UUID
	err = unit.ExecuteWithRepositoryProvider(operationCtx, func(_ *shoppingdb.RepositoryProvider) error {
		created, err := service.CreateList(operationCtx, owner, "Rollback")
		if err != nil {
			return err
		}
		rollbackID = created.ID()
		return failure
	})
	require.ErrorIs(t, err, failure)
	_, err = service.GetList(ctx, owner, rollbackID)
	require.ErrorIs(t, err, domain.ErrListNotFound)
	var count int
	require.NoError(
		t,
		client.QueryRowContext(ctx, "test.outbox.count", `SELECT COUNT(*) FROM event_outbox`).Scan(&count),
	)
	require.Equal(t, 2, count)
	testListOwnerGRPC(t, service, owner, stranger, list.ID())
	testShortageInbox(ctx, t, client, unit, service, owner, list.ID())
}

func testListOwnerGRPC(t *testing.T, service *app.Service, owner, stranger, list uuid.UUID) {
	t.Helper()
	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	shoppingapi.RegisterShoppingServiceServer(server, handler.NewHandler(service, slog.Default()))
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient(
		"passthrough:///shopping",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	client := shoppingapi.NewShoppingServiceClient(conn)
	request := &shoppingapi.IsListOwnerRequest{UserId: owner.String(), ListId: list.String()}
	result, err := client.IsListOwner(t.Context(), request)
	require.NoError(t, err)
	require.True(t, result.GetIsOwner())
	request.UserId = stranger.String()
	result, err = client.IsListOwner(t.Context(), request)
	require.NoError(t, err)
	require.False(t, result.GetIsOwner())
	request.UserId = "invalid"
	_, err = client.IsListOwner(t.Context(), request)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

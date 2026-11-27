package integration_tests

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/amqp"
	"github.com/distributed-programming-2026/go-sdk/pkg/event"
	eventamqp "github.com/distributed-programming-2026/go-sdk/pkg/event/amqp"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"

	domainevent "github.com/distributed-programming-2026/lib/event"

	internalapi "audit/api/server/internalapi"
	publicapi "audit/api/server/publicapi"
	inventoryevents "inventory/api/events"
)

func TestHTTPAndGRPCDelivery(t *testing.T) {
	client, err := publicapi.NewClientWithResponses("http://" + testEnv.conf.HTTP.Address)
	require.NoError(t, err)

	response, err := client.EchoWithResponse(t.Context(), publicapi.EchoJSONRequestBody{
		Body: "HTTP привет",
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode())
	require.NotNil(t, response.JSON200)
	envelope := checkEcho(t, response.JSON200.OperationID, "HTTP привет")
	waitHandled(t, envelope, 1)

	connection, err := grpc.NewClient(
		testEnv.conf.GRPC.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer connection.Close()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	grpcResponse, err := internalapi.NewAuditServiceClient(connection).Echo(ctx, &internalapi.EchoRequest{
		Body: "gRPC echo",
	}, grpc.WaitForReady(true))
	require.NoError(t, err)
	waitHandled(t, checkEcho(t, grpcResponse.OperationID, "gRPC echo"), 1)

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	duplicateConnection := amqp.NewConnection(moduleID, amqp.ConnectionConfig{
		URL:            testEnv.conf.AMQP.URL,
		ConnectTimeout: time.Second,
	}, logger)
	defer func() {
		require.NoError(t, duplicateConnection.Stop())
	}()

	dispatcher, err := eventamqp.NewDispatcher(duplicateConnection.Producer(domainevent.DomainExchange(), nil, nil))
	require.NoError(t, err)
	require.NoError(t, duplicateConnection.Start())
	require.NoError(t, dispatcher.Dispatch(t.Context(), envelope))
	waitHandled(t, envelope, 2)
	require.Equal(t, 1, testEnv.logs.count("echo received", "echo_id", response.JSON200.OperationID))
}

func TestInventoryEventIsAuditedOnce(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	connection := amqp.NewConnection(moduleID, amqp.ConnectionConfig{
		URL: testEnv.conf.AMQP.URL, ConnectTimeout: time.Second,
	}, logger)
	defer func() { require.NoError(t, connection.Stop()) }()
	dispatcher, err := eventamqp.NewDispatcher(connection.Producer(domainevent.DomainExchange(), nil, nil))
	require.NoError(t, err)
	require.NoError(t, connection.Start())
	actor, inventoryID := uuid.New(), uuid.New()
	envelope, err := event.New(t.Context(), "inventory", "InventoryCreated", &inventoryevents.InventoryCreated{
		InventoryId: inventoryID.String(), Name: "Морозилка", ActorId: actor.String(),
	})
	require.NoError(t, err)
	require.NoError(t, dispatcher.Dispatch(t.Context(), envelope))
	require.NoError(t, dispatcher.Dispatch(t.Context(), envelope))
	waitHandled(t, envelope, 2)
	var action, target, correlation string
	var actorBytes []byte
	err = testEnv.connector.TransactionalClient().QueryRowContext(t.Context(), "test.audit.event",
		`SELECT action, target_id, correlation_id, actor_id FROM audit_entries WHERE event_id = ?`, envelope.ID(),
	).Scan(&action, &target, &correlation, &actorBytes)
	require.NoError(t, err)
	require.Equal(t, "inventory.created", action)
	require.Equal(t, inventoryID.String(), target)
	require.Equal(t, envelope.CorrelationID(), correlation)
	require.Equal(t, actor[:], actorBytes)
}

func TestReservationEventsAreAudited(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	connection := amqp.NewConnection(moduleID, amqp.ConnectionConfig{
		URL: testEnv.conf.AMQP.URL, ConnectTimeout: time.Second,
	}, logger)
	defer func() { require.NoError(t, connection.Stop()) }()
	dispatcher, err := eventamqp.NewDispatcher(connection.Producer(domainevent.DomainExchange(), nil, nil))
	require.NoError(t, err)
	require.NoError(t, connection.Start())
	inventoryID, reservationID, otherID, productID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	events := []struct {
		name, action string
		target       string
		payload      proto.Message
	}{
		{"ReservationCreated", "reservation.created", reservationID.String(), &inventoryevents.ReservationCreated{
			InventoryId: inventoryID.String(), ReservationId: reservationID.String(),
			Items: []*inventoryevents.ReservationLine{{ProductId: productID.String(), Quantity: 2}},
		}},
		{"ReservationCancelled", "reservation.cancelled", reservationID.String(), &inventoryevents.ReservationCancelled{
			InventoryId: inventoryID.String(), ReservationId: reservationID.String(),
		}},
		{"ReservationConsumed", "reservation.consumed", otherID.String(), &inventoryevents.ReservationConsumed{
			InventoryId: inventoryID.String(), ReservationId: otherID.String(),
		}},
	}
	for _, item := range events {
		envelope, err := event.New(t.Context(), "inventory", item.name, item.payload)
		require.NoError(t, err)
		require.NoError(t, dispatcher.Dispatch(t.Context(), envelope))
		waitHandled(t, envelope, 1)
		var action, target string
		err = testEnv.connector.TransactionalClient().QueryRowContext(t.Context(), "test.audit.reservation",
			`SELECT action, target_id FROM audit_entries WHERE event_id = ?`, envelope.ID()).Scan(&action, &target)
		require.NoError(t, err)
		require.Equal(t, item.action, action)
		require.Equal(t, item.target, target)
	}
}

func TestInvalidHTTPRequests(t *testing.T) {
	client := &http.Client{
		Timeout: time.Second,
	}
	for _, body := range []string{`{}`, `{"body":42}`, `null`, `{"body":`} {
		request, err := http.NewRequestWithContext(t.Context(), http.MethodPost,
			"http://"+testEnv.conf.HTTP.Address+"/api/public/v1/echo", strings.NewReader(body),
		)
		require.NoError(t, err)
		request.Header.Set("Content-Type", "application/json")

		response, err := client.Do(request)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		require.Equal(t, http.StatusBadRequest, response.StatusCode)
	}
}

func TestAuditBackendFailsClosedWithoutAuthz(t *testing.T) {
	client := &http.Client{Timeout: 6 * time.Second}
	for _, test := range []struct {
		name   string
		token  string
		status int
	}{
		{name: "missing session", status: http.StatusUnauthorized},
		{
			name:   "authz unavailable",
			token:  "unverified",
			status: http.StatusServiceUnavailable,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			request, err := http.NewRequestWithContext(t.Context(), http.MethodGet,
				"http://"+testEnv.conf.HTTP.Address+"/api/public/v1/audit/entries", http.NoBody)
			require.NoError(t, err)
			request.Header.Set("X-User-ID", "spoofed")
			if test.token != "" {
				request.Header.Set("Cookie", "session="+test.token)
			}
			response, err := client.Do(request)
			require.NoError(t, err)
			require.NoError(t, response.Body.Close())
			require.Equal(t, test.status, response.StatusCode)
		})
	}
}

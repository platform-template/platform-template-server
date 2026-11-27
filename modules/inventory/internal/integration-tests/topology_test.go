package integration_tests

import (
	"io"
	"log/slog"
	"testing"
	"time"
	"uuid"

	mealplannerevents "mealplanner/api/events"

	"github.com/distributed-programming-2026/go-sdk/pkg/amqp"
	"github.com/distributed-programming-2026/go-sdk/pkg/event"
	eventamqp "github.com/distributed-programming-2026/go-sdk/pkg/event/amqp"
	domainevent "github.com/distributed-programming-2026/lib/event"
	rabbit "github.com/rabbitmq/amqp091-go"
	"github.com/stretchr/testify/require"
)

func TestSubscriberQueueExistsBeforeConsumerStarts(t *testing.T) {
	queues := []string{"test.offline." + uuid.New().String(), "test.offline." + uuid.New().String()}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	connection := amqp.NewConnection(
		"topology-test",
		amqp.ConnectionConfig{URL: testEnv.conf.AMQP.URL, ConnectTimeout: time.Second},
		logger,
	)
	for _, queue := range queues {
		domainevent.DeclareSubscriber(connection, queue, "topologytest.MealPlanningRequested")
	}
	producer := connection.Producer(domainevent.DomainExchange(), nil, nil)
	dispatcher, err := eventamqp.NewDispatcher(producer)
	require.NoError(t, err)
	require.NoError(t, connection.Start())
	envelope, err := event.New(
		t.Context(),
		"topologytest",
		"MealPlanningRequested",
		&mealplannerevents.MealPlanningRequested{ItemId: uuid.New().String()},
	)
	require.NoError(t, err)
	require.NoError(t, dispatcher.Dispatch(t.Context(), envelope))
	require.NoError(t, connection.Stop())
	broker, err := rabbit.Dial(testEnv.conf.AMQP.URL)
	require.NoError(t, err)
	defer broker.Close()
	channel, err := broker.Channel()
	require.NoError(t, err)
	defer channel.Close()
	for _, name := range []string{testEnv.conf.AMQP.MealPlannerQueue, testEnv.conf.AMQP.ShoppingQueue, testEnv.conf.AMQP.AuditQueue} {
		_, err := channel.QueueDeclarePassive(name, true, false, false, false, nil)
		require.NoError(t, err)
	}
	for _, queue := range queues {
		delivery, ok, err := channel.Get(queue, true)
		require.NoError(t, err)
		require.True(t, ok)
		received, err := event.Unmarshal(delivery.Body)
		require.NoError(t, err)
		require.Equal(t, envelope.ID(), received.ID())
		_, err = channel.QueueDelete(queue, false, false, false)
		require.NoError(t, err)
		_, err = channel.QueueDelete(queue+".dead", false, false, false)
		require.NoError(t, err)
	}
}

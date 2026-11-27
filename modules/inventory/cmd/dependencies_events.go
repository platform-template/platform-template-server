package main

import (
	"log/slog"

	"github.com/distributed-programming-2026/go-sdk/pkg/amqp"
	"github.com/distributed-programming-2026/go-sdk/pkg/event"
	eventamqp "github.com/distributed-programming-2026/go-sdk/pkg/event/amqp"
	"github.com/distributed-programming-2026/go-sdk/pkg/event/outbox"
	"github.com/distributed-programming-2026/go-sdk/pkg/uow"

	domainevent "github.com/distributed-programming-2026/lib/event"

	"inventory/internal/inventory/app"
	"inventory/internal/inventory/config"
	eventinfra "inventory/internal/inventory/infra/event"
	eventhandler "inventory/internal/inventory/infra/handlers/events"
)

func newDispatcher(
	logger *slog.Logger,
	conf config.Env,
	db *database,
	connection amqp.Connection,
) (event.Dispatcher, *outbox.Relay, error) {
	domainevent.DeclareSubscriber(
		connection,
		conf.AMQP.MealPlannerQueue,
		"inventory.AllocationCompleted",
		"inventory.AllocationCancelled",
	)
	domainevent.DeclareSubscriber(connection, conf.AMQP.ShoppingQueue, "inventory.AllocationCompleted")
	domainevent.DeclareSubscriber(connection, conf.AMQP.AuditQueue, "*.AuditRecordRequested")

	// Declare the durable queue before publishing, even if the consumer is offline.
	producer := connection.Producer(domainevent.DomainExchange(), eventQueue(conf), eventBinding(conf))
	transport, err := eventamqp.NewDispatcher(producer)
	if err != nil {
		return nil, nil, err
	}

	return outbox.Decorate(
		event.ChainDispatcher(transport, event.DispatcherLogging(logger)),
		uow.NewLockableUnitOfWork(db.unit),
		outbox.Config{
			Transport:   "rabbitmq",
			BatchSize:   conf.Relay.BatchSize,
			LockTimeout: conf.Relay.LockTimeout,
		},
	)
}

func newMessageHandler(
	logger *slog.Logger,
	conf config.Env,
	db *database,
	dispatcher event.Dispatcher,
) (amqp.Handler, error) {
	svc := app.NewService(db.unit, eventinfra.NewDispatcherFactory(appID, dispatcher))
	handler, err := eventhandler.WithInbox(eventhandler.NewHandler(logger, svc), db.unit, conf.AMQP.Queue)
	if err != nil {
		return nil, err
	}

	return eventamqp.NewHandler(event.ChainHandler(handler, event.HandlerLogging(logger)))
}

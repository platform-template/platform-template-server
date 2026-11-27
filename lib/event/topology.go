package event

import "github.com/distributed-programming-2026/go-sdk/pkg/amqp"

// DeclareSubscriber registers a durable queue and binding before the connection starts.
// The same queue and dead-letter settings must be used by its consumer.
func DeclareSubscriber(connection amqp.Connection, queueName string, routingKeys ...string) {
	queue := &amqp.QueueConfig{
		Name:    queueName,
		Durable: true,
		DLQ: &amqp.DLQConfig{
			Exchange:   *DomainDeadLetterExchange(),
			Queue:      queueName + ".dead",
			RoutingKey: queueName,
			Durable:    true,
		},
	}
	binding := &amqp.BindConfig{QueueName: queueName, ExchangeName: DomainExchange().Name, RoutingKeys: routingKeys}
	connection.Producer(DomainExchange(), queue, binding)
}

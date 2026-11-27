package events

import (
	"context"
	"errors"

	"github.com/distributed-programming-2026/go-sdk/pkg/event"
	"github.com/distributed-programming-2026/go-sdk/pkg/event/inbox"
	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
)

type transactionContextKey struct{}
type transactionContext struct{ ctx context.Context }

// The SDK Inbox derives a context before invoking its handler, while its UOW
// shares transactions by exact context identity. Preserve the Inbox context for
// nested operations; the incoming AMQP context already carries correlation data.
func WithInbox(next event.Handler, unit uow.UnitOfWork, consumer string) (event.Handler, error) {
	if next == nil {
		return nil, errors.New("inventory event handler is nil")
	}
	inside := event.HandlerFunc(func(ctx context.Context, envelope event.Envelope) error {
		transaction, ok := ctx.Value(transactionContextKey{}).(*transactionContext)
		if !ok || transaction.ctx == nil {
			return errors.New("missing inbox transaction context")
		}
		return next.Handle(transaction.ctx, envelope)
	})
	wrapped, _, err := inbox.Decorate(inside, unit, inbox.Config{Consumer: consumer})
	if err != nil {
		return nil, err
	}
	return event.HandlerFunc(func(ctx context.Context, envelope event.Envelope) error {
		transaction := &transactionContext{}
		txCtx := context.WithValue(ctx, transactionContextKey{}, transaction)
		transaction.ctx = txCtx
		return wrapped.Handle(txCtx, envelope)
	}), nil
}

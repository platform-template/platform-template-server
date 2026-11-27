package app

import (
	"context"
	"errors"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	"google.golang.org/protobuf/proto"

	domainevent "github.com/distributed-programming-2026/lib/event"

	"inventory/internal/inventory/domain"
	"inventory/internal/inventory/infra/mysql"
)

type DispatcherFactory interface {
	NewDispatcher(ctx context.Context) domainevent.Dispatcher
}

type Service struct {
	unit              uow.UnitOfWorkWithRepositoryProvider[*mysql.RepositoryProvider]
	dispatcherFactory DispatcherFactory
}

type actorKey struct{}

func WithActor(ctx context.Context, id uuid.UUID) context.Context {
	return context.WithValue(ctx, actorKey{}, id)
}

func actorID(ctx context.Context) string {
	id, _ := ctx.Value(actorKey{}).(uuid.UUID)
	if id == uuid.Nil() {
		return ""
	}
	return id.String()
}

func (s *Service) dispatch(ctx context.Context, message proto.Message) error {
	if s.dispatcherFactory == nil {
		return errors.New("inventory event dispatcher is not configured")
	}
	dispatcher := s.dispatcherFactory.NewDispatcher(ctx)
	if err := dispatcher.Dispatch(message); err != nil {
		return err
	}
	record, err := auditRecord(message)
	if err != nil {
		return err
	}
	return dispatcher.Dispatch(record)
}

func NewService(
	unit uow.UnitOfWorkWithRepositoryProvider[*mysql.RepositoryProvider],
	dispatcherFactory DispatcherFactory,
) *Service {
	return &Service{
		unit:              unit,
		dispatcherFactory: dispatcherFactory,
	}
}

func (s *Service) Echo(ctx context.Context, body string) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.unit.ExecuteWithRepositoryProvider(ctx, func(provider *mysql.RepositoryProvider) error {
		svc := domain.NewEchoService(
			provider.Echo(ctx),
			s.dispatcherFactory.NewDispatcher(ctx),
		)

		var err error
		id, err = svc.Echo(body)
		return err
	})
	if err != nil {
		return uuid.Nil(), err
	}
	return id, nil
}

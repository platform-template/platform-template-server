package mysql

import (
	"context"
	"notifications/internal/notifications/domain"
	"notifications/internal/notifications/infra/mysql/repository"

	"github.com/distributed-programming-2026/go-sdk/pkg/mysql"
)

type RepositoryProvider struct{ client mysql.ClientContext }

func NewRepositoryProvider(client mysql.ClientContext) *RepositoryProvider {
	return &RepositoryProvider{client: client}
}
func (p *RepositoryProvider) Notification(ctx context.Context) domain.NotificationRepository {
	return repository.New(ctx, p.client)
}

func (p *RepositoryProvider) Echo(ctx context.Context) domain.EchoRepository {
	return repository.NewEchoRepository(ctx, p.client)
}

package app

import (
	"context"
	"errors"
	"testing"
	"uuid"

	"github.com/distributed-programming-2026/go-sdk/pkg/uow"
	"github.com/stretchr/testify/require"

	"notifications/internal/notifications/domain"
	"notifications/internal/notifications/infra/mysql"
)

type failingUnit struct {
	uow.UnitOfWorkWithRepositoryProvider[*mysql.RepositoryProvider]
	err error
}

func (u failingUnit) ExecuteWithRepositoryProvider(_ context.Context, _ func(*mysql.RepositoryProvider) error) error {
	return u.err
}

func TestListValidatesPaginationAndUser(t *testing.T) {
	svc := NewService(failingUnit{}, nil)
	tests := []struct {
		name    string
		user    uuid.UUID
		options domain.ListOptions
	}{
		{
			name:    "empty user",
			user:    uuid.Nil(),
			options: domain.ListOptions{Limit: 20},
		},
		{
			name:    "negative limit",
			user:    uuid.New(),
			options: domain.ListOptions{Limit: -1},
		},
		{
			name:    "excess limit",
			user:    uuid.New(),
			options: domain.ListOptions{Limit: 1001},
		},
		{
			name:    "negative offset",
			user:    uuid.New(),
			options: domain.ListOptions{Offset: -1},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := svc.ListNotifications(t.Context(), test.user, test.options)
			require.ErrorIs(t, err, domain.ErrInvalidNotification)
		})
	}
}

func TestCreateValidatesAndPreservesUnitErrors(t *testing.T) {
	duplicate := NewService(failingUnit{err: domain.ErrDuplicate}, nil)
	input := CreateNotificationInput{
		UserID: uuid.New(),
		Kind:   "access.granted",
		Title:  "Title",
	}
	_, err := duplicate.CreateNotification(t.Context(), input)
	require.ErrorIs(t, err, domain.ErrDuplicate)

	input.Title = " "
	_, err = duplicate.CreateNotification(t.Context(), input)
	require.ErrorIs(t, err, domain.ErrInvalidNotification)

	input.Title = "Title"
	input.Kind = "invalid-kind"
	_, err = duplicate.CreateNotification(t.Context(), input)
	require.ErrorIs(t, err, domain.ErrInvalidKind)
}

func TestOperationsPreserveUnitErrors(t *testing.T) {
	expected := errors.New("transaction failed")
	svc := NewService(failingUnit{err: expected}, nil)
	userID := uuid.New()

	_, err := svc.ListNotifications(t.Context(), userID, domain.ListOptions{})
	require.ErrorIs(t, err, expected)

	_, err = svc.GetUnreadCount(t.Context(), userID)
	require.ErrorIs(t, err, expected)

	err = svc.MarkAsRead(t.Context(), userID, uuid.New())
	require.ErrorIs(t, err, expected)

	svc = NewService(failingUnit{err: domain.ErrNotFound}, nil)
	err = svc.MarkAsRead(t.Context(), userID, uuid.New())
	require.ErrorIs(t, err, domain.ErrNotFound)
}

package domain

import (
	"strings"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/require"
)

func TestNotificationValidation(t *testing.T) {
	valid := NotificationData{
		ID:        uuid.New(),
		UserID:    uuid.New(),
		Kind:      "access.granted",
		Title:     "Title",
		CreatedAt: time.Now(),
	}
	tests := []struct {
		name   string
		change func(*NotificationData)
	}{
		{"id", func(d *NotificationData) { d.ID = uuid.Nil() }},
		{"user", func(d *NotificationData) { d.UserID = uuid.Nil() }},
		{"empty kind", func(d *NotificationData) { d.Kind = "" }},
		{"invalid kind", func(d *NotificationData) { d.Kind = "bad-kind" }},
		{"kind length", func(d *NotificationData) {
			d.Kind = NotificationKind(strings.Repeat("a", 129))
		}},
		{"title", func(d *NotificationData) { d.Title = "  " }},
		{"title length", func(d *NotificationData) { d.Title = strings.Repeat("a", 256) }},
		{"body length", func(d *NotificationData) { d.Body = strings.Repeat("a", 10001) }},
		{"time", func(d *NotificationData) { d.CreatedAt = time.Time{} }},
		{"empty key", func(d *NotificationData) {
			key := "  "
			d.IdempotencyKey = &key
		}},
		{"key length", func(d *NotificationData) {
			key := strings.Repeat("a", 256)
			d.IdempotencyKey = &key
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := valid
			tt.change(&data)

			_, err := NewNotification(data)

			require.Error(t, err)
		})
	}

	n, err := NewNotification(valid)

	require.NoError(t, err)
	require.Nil(t, n.Data().ReadAt)
	require.Nil(t, n.Data().IdempotencyKey)

	kind, err := NewNotificationKind(" Access.Granted ")

	require.NoError(t, err)
	require.Equal(t, NotificationKind("access.granted"), kind)
}

func TestReadAndRestore(t *testing.T) {
	created := time.Now().UTC().Truncate(time.Microsecond)
	key := "event:1"
	n, err := NewNotification(
		NotificationData{
			ID:             uuid.New(),
			UserID:         uuid.New(),
			Kind:           "system.announcement",
			Title:          "Title",
			CreatedAt:      created,
			IdempotencyKey: &key,
		},
	)
	require.NoError(t, err)

	err = n.MarkAsRead(created.Add(-time.Second))
	require.ErrorIs(t, err, ErrInvalidReadAt)

	err = n.MarkAsRead(time.Time{})
	require.ErrorIs(t, err, ErrInvalidReadAt)

	read := created.Add(time.Second)
	err = n.MarkAsRead(read)
	require.NoError(t, err)

	err = n.MarkAsRead(read.Add(time.Hour))
	require.NoError(t, err)
	require.Equal(t, read, *n.Data().ReadAt)

	restored, err := RestoreNotification(n.Data())
	require.NoError(t, err)
	require.Equal(t, n.Data(), restored.Data())

	data := n.Data()
	bad := created.Add(-time.Second)
	data.ReadAt = &bad

	_, err = RestoreNotification(data)
	require.ErrorIs(t, err, ErrInvalidReadAt)
}

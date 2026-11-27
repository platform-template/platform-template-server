package app

import (
	"context"
	"errors"
	"testing"
	"uuid"

	"github.com/stretchr/testify/require"
)

const inventoryModule = "inventory"

type fakeSessions struct {
	user  uuid.UUID
	err   error
	token string
}

func (f *fakeSessions) Resolve(_ context.Context, token string) (uuid.UUID, error) {
	f.token = token
	return f.user, f.err
}

type fakeAccess struct {
	modules []string
	super   bool
	err     error
	token   string
	user    uuid.UUID
}

func (f *fakeAccess) List(_ context.Context, token string, user uuid.UUID) (modules []string, superuser bool, err error) {
	f.token, f.user = token, user
	return append([]string(nil), f.modules...), f.super, f.err
}

func TestBootstrapAndNavigation(t *testing.T) {
	user := uuid.New()
	sessions := &fakeSessions{user: user}
	access := &fakeAccess{modules: []string{"shopping", inventoryModule}}
	service := NewBootstrapService(sessions, access)
	bootstrap, err := service.GetBootstrap(t.Context(), "session-token")
	require.NoError(t, err)
	require.Equal(t, user, bootstrap.UserID)
	require.Equal(t, []string{inventoryModule, "shopping"}, bootstrap.Modules)
	require.Equal(t, "session-token", sessions.token)
	require.Equal(t, "session-token", access.token)
	items, err := service.GetNavigation(t.Context(), "session-token")
	require.NoError(t, err)
	require.Equal(t, []NavigationItem{
		{ID: inventoryModule, Label: "Инвентари", Path: "/inventory"},
		{ID: "shopping", Label: "Покупки", Path: "/shopping"},
	}, items)
}

func TestBootstrapRejectsMissingOrInvalidSession(t *testing.T) {
	service := NewBootstrapService(&fakeSessions{err: ErrUnauthenticated}, &fakeAccess{})
	_, err := service.GetBootstrap(t.Context(), "")
	require.ErrorIs(t, err, ErrUnauthenticated)
	_, err = service.GetBootstrap(t.Context(), "invalid")
	require.ErrorIs(t, err, ErrUnauthenticated)
	service = NewBootstrapService(&fakeSessions{user: uuid.New()}, &fakeAccess{err: errors.New("offline")})
	_, err = service.GetBootstrap(t.Context(), "valid")
	require.Error(t, err)
}

func TestSuperuserNavigationContainsAllModules(t *testing.T) {
	service := NewBootstrapService(&fakeSessions{user: uuid.New()}, &fakeAccess{super: true})
	items, err := service.GetNavigation(t.Context(), "session-token")
	require.NoError(t, err)
	require.Len(t, items, len(moduleNavigation)+1)
	require.Equal(t, "admin", items[len(items)-1].ID)
}

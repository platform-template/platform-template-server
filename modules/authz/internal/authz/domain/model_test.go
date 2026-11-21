package domain

import (
	"errors"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/require"
)

func TestNewModuleGrant(t *testing.T) {
	userID := uuid.New()
	grantedBy := uuid.New()
	moduleKey, err := NewModuleKey(" Meal_Planner ")
	require.NoError(t, err)

	grantedAt := time.Date(
		2026,
		11,
		21,
		12,
		0,
		0,
		0,
		time.FixedZone("UTC+3", 3*60*60),
	)
	grant, err := NewModuleGrant(userID, moduleKey, &grantedBy, grantedAt)
	require.NoError(t, err)

	grantedBy = uuid.Nil()
	data := grant.Data()
	require.Equal(t, userID, data.UserID)
	require.Equal(t, ModuleKey("meal_planner"), data.ModuleKey)
	require.NotNil(t, data.GrantedBy)
	require.NotEqual(t, uuid.Nil(), *data.GrantedBy)
	require.Equal(t, grantedAt.UTC(), data.GrantedAt)
}

func TestNewModuleKeyRejectsEmptyAndInvalidValues(t *testing.T) {
	for _, value := range []string{"", "   ", "meal-planner", "1inventory", "meal planner"} {
		_, err := NewModuleKey(value)
		require.ErrorIs(t, err, ErrInvalidModuleKey)
	}
}

func TestNewModuleGrantValidatesItsInvariants(t *testing.T) {
	moduleKey, err := NewModuleKey("inventory")
	require.NoError(t, err)

	_, err = NewModuleGrant(uuid.Nil(), moduleKey, nil, time.Now())
	require.ErrorIs(t, err, ErrInvalidUserID)

	nilIssuer := uuid.Nil()
	_, err = NewModuleGrant(uuid.New(), moduleKey, &nilIssuer, time.Now())
	require.ErrorIs(t, err, ErrInvalidGrantedBy)

	_, err = NewModuleGrant(uuid.New(), moduleKey, nil, time.Time{})
	require.ErrorIs(t, err, ErrInvalidGrantedAt)

	_, err = NewModuleGrant(
		uuid.New(),
		ModuleKey("not valid"),
		nil,
		time.Now(),
	)
	require.ErrorIs(t, err, ErrInvalidModuleKey)
}

func TestSystemRoles(t *testing.T) {
	superuser, err := NewSystemRole("superuser")
	require.NoError(t, err)
	require.True(t, superuser.HasGlobalPrivileges())

	user, err := NewSystemRole("user")
	require.NoError(t, err)
	require.False(t, user.HasGlobalPrivileges())

	_, err = NewSystemRole("administrator")
	require.ErrorIs(t, err, ErrInvalidSystemRole)

	assignment, err := NewSystemRoleAssignment(uuid.New(), superuser)
	require.NoError(t, err)
	require.Equal(t, superuser, assignment.Data().Role)

	_, err = NewSystemRoleAssignment(uuid.New(), user)
	require.ErrorIs(t, err, ErrDefaultRoleAssignment)
	_, err = NewSystemRoleAssignment(uuid.Nil(), superuser)
	require.True(t, errors.Is(err, ErrInvalidUserID))
}

func TestEntityRestorationUsesTheSameInvariants(t *testing.T) {
	_, err := NewModuleGrant(
		uuid.Nil(),
		ModuleKey("inventory"),
		nil,
		time.Now(),
	)
	require.ErrorIs(t, err, ErrInvalidUserID)

	_, err = NewSystemRoleAssignment(uuid.New(), SystemRole("broken"))
	require.ErrorIs(t, err, ErrInvalidSystemRole)
}

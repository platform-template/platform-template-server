package internalapi

import (
	"context"
	"testing"
	"uuid"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	internalapi "authz/api/server/internalapi"
	"authz/internal/authz/domain"
)

type fakeService struct{ superuser bool }

func (s fakeService) GrantAccess(
	context.Context,
	uuid.UUID,
	uuid.UUID,
	string,
) error {
	return nil
}

func (s fakeService) RevokeAccess(
	context.Context,
	uuid.UUID,
	uuid.UUID,
	string,
) error {
	return nil
}

func (s fakeService) CheckAccess(
	context.Context,
	uuid.UUID,
	string,
) (bool, error) {
	return false, nil
}

func (s fakeService) ListAccess(
	context.Context,
	uuid.UUID,
) ([]domain.ModuleGrantData, bool, error) {
	return nil, false, nil
}

func (s fakeService) IsSuperuser(
	context.Context,
	uuid.UUID,
) (bool, error) {
	return s.superuser, nil
}

type fakeSessions struct{ userID uuid.UUID }

func (s fakeSessions) Resolve(
	context.Context,
	string,
) (uuid.UUID, error) {
	return s.userID, nil
}

func TestCheckRequiresSession(t *testing.T) {
	handler := NewHandler(
		fakeService{},
		fakeSessions{userID: uuid.New()},
		nil,
	)
	_, err := handler.CheckAccess(
		t.Context(),
		&internalapi.CheckAccessRequest{
			UserId:    uuid.New().String(),
			ModuleKey: "inventory",
		},
	)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestCheckDoesNotAllowForeignUserForRegularActor(t *testing.T) {
	handler := NewHandler(
		fakeService{},
		fakeSessions{userID: uuid.New()},
		nil,
	)
	ctx := metadata.NewIncomingContext(
		t.Context(),
		metadata.Pairs("session", "token"),
	)
	_, err := handler.CheckAccess(
		ctx,
		&internalapi.CheckAccessRequest{
			UserId:    uuid.New().String(),
			ModuleKey: "inventory",
		},
	)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
}

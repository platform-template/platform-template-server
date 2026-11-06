package identity

import (
	"context"
	"testing"
	"uuid"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	identityapi "identity/api/server/internalapi"
)

type registrationRPCMock struct {
	identityapi.IdentityServiceClient

	prepareRequest  *identityapi.PrepareRegistrationRequest
	commitRequest   *identityapi.RegistrationTransactionRequest
	rollbackRequest *identityapi.RegistrationTransactionRequest
}

func (f *registrationRPCMock) PrepareRegistration(
	_ context.Context,
	req *identityapi.PrepareRegistrationRequest,
	_ ...grpc.CallOption,
) (*identityapi.RegistrationTransactionResponse, error) {
	f.prepareRequest = req
	return &identityapi.RegistrationTransactionResponse{}, nil
}

func (f *registrationRPCMock) CommitRegistration(
	_ context.Context,
	req *identityapi.RegistrationTransactionRequest,
	_ ...grpc.CallOption,
) (*identityapi.RegistrationTransactionResponse, error) {
	f.commitRequest = req
	return &identityapi.RegistrationTransactionResponse{}, nil
}

func (f *registrationRPCMock) RollbackRegistration(
	_ context.Context,
	req *identityapi.RegistrationTransactionRequest,
	_ ...grpc.CallOption,
) (*identityapi.RegistrationTransactionResponse, error) {
	f.rollbackRequest = req
	return &identityapi.RegistrationTransactionResponse{}, nil
}

func TestRegistrationClient(t *testing.T) {
	rpc := &registrationRPCMock{}
	client := &Client{client: rpc}

	transactionID := uuid.New()
	userID := uuid.New()

	err := client.Prepare(
		t.Context(),
		transactionID,
		userID,
		"Klava",
		"Koka",
		"",
		"cockhuh",
		"koka@example.com",
	)
	require.NoError(t, err)

	require.NotNil(t, rpc.prepareRequest)
	require.Equal(t, transactionID.String(), rpc.prepareRequest.GetTransactionId())
	require.Equal(t, userID.String(), rpc.prepareRequest.GetUserId())
	require.Equal(t, "cockhuh", rpc.prepareRequest.GetLogin())
	require.Nil(t, rpc.prepareRequest.MiddleName)

	err = client.Commit(t.Context(), transactionID)
	require.NoError(t, err)

	require.NotNil(t, rpc.commitRequest)
	require.Equal(t, transactionID.String(), rpc.commitRequest.GetTransactionId())

	err = client.Rollback(t.Context(), transactionID)
	require.NoError(t, err)

	require.NotNil(t, rpc.rollbackRequest)
	require.Equal(t, transactionID.String(), rpc.rollbackRequest.GetTransactionId())
}

package app

import (
	"errors"
	"slices"
	"testing"
	"uuid"
)

func TestRecoverCommits(t *testing.T) {
	failure := errors.New("simulated failure")

	tests := []struct {
		name              string
		state             RegistrationState
		identityCommitErr error
		wantState         RegistrationState
		wantErr           bool
		wantCalls         []string
	}{
		{
			name:      "recover committed decision",
			state:     RegistrationCommitDecided,
			wantState: RegistrationCommitted,
			wantCalls: []string{
				"auth.commit",
				"identity.commit",
				"journal.mark_committed",
			},
		},
		{
			name:              "identity commit still fails",
			state:             RegistrationCommitDecided,
			identityCommitErr: failure,
			wantState:         RegistrationCommitDecided,
			wantErr:           true,
			wantCalls: []string{
				"auth.commit",
				"identity.commit",
			},
		},
		{
			name:      "preparing is not recovered",
			state:     RegistrationPreparing,
			wantState: RegistrationPreparing,
			wantErr:   true,
		},
		{
			name:      "abort is not recovered yet",
			state:     RegistrationAbortDecided,
			wantState: RegistrationAbortDecided,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string

			transactionID := uuid.New()
			userID := uuid.New()

			journal := &coordinatorJournalMock{
				calls: &calls,
				state: tt.state,
				pending: []RegistrationTransaction{
					{
						TransactionID: transactionID,
						UserID:        userID,
						State:         tt.state,
					},
				},
			}

			auth := &coordinatorAuthMock{
				coordinatorBranchMock: &coordinatorBranchMock{
					name:  "auth",
					calls: &calls,
				},
			}

			identity := &coordinatorIdentityMock{
				coordinatorBranchMock: &coordinatorBranchMock{
					name:      "identity",
					calls:     &calls,
					commitErr: tt.identityCommitErr,
				},
			}

			coordinator := NewRegistrationCoordinator(
				journal,
				auth,
				identity,
			)

			err := coordinator.RecoverCommits(t.Context())

			if (err != nil) != tt.wantErr {
				t.Fatalf("unexpected error: %v", err)
			}

			if journal.state != tt.wantState {
				t.Errorf(
					"state: got %s, want %s",
					journal.state,
					tt.wantState,
				)
			}

			if !slices.Equal(calls, tt.wantCalls) {
				t.Errorf(
					"calls: got %v, want %v",
					calls,
					tt.wantCalls,
				)
			}
		})
	}
}

package app

import (
	"context"
	"errors"
	"slices"
	"testing"
	"uuid"
)

func TestRecoverPending(t *testing.T) {
	failure := errors.New("simulated failure")

	tests := []struct {
		name                string
		state               RegistrationState
		identityCommitErr   error
		wantState           RegistrationState
		wantErr             bool
		wantCalls           []string
		authRollbackErr     error
		identityRollbackErr error
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
			name:      "recover preparing transaction",
			state:     RegistrationPreparing,
			wantState: RegistrationAborted,
			wantCalls: []string{
				"journal.decide_abort",
				"auth.rollback",
				"identity.rollback",
				"journal.mark_aborted",
			},
		},
		{
			name:      "recover aborted decision",
			state:     RegistrationAbortDecided,
			wantState: RegistrationAborted,
			wantCalls: []string{
				"auth.rollback",
				"identity.rollback",
				"journal.mark_aborted",
			},
		},
		{
			name:                "identity rollback failed",
			state:               RegistrationAbortDecided,
			identityRollbackErr: failure,
			wantState:           RegistrationAbortDecided,
			wantErr:             true,
			wantCalls: []string{
				"auth.rollback",
				"identity.rollback",
			},
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
					name:        "auth",
					calls:       &calls,
					rollbackErr: tt.authRollbackErr,
				},
			}

			identity := &coordinatorIdentityMock{
				coordinatorBranchMock: &coordinatorBranchMock{
					name:        "identity",
					calls:       &calls,
					commitErr:   tt.identityCommitErr,
					rollbackErr: tt.identityRollbackErr,
				},
			}

			coordinator := NewRegistrationCoordinator(
				journal,
				auth,
				identity,
			)

			err := coordinator.RecoverPending(t.Context())

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

func TestRecoverPendingJournalError(t *testing.T) {
	failure := errors.New("database unavailable")
	var calls []string

	journal := &coordinatorJournalMock{
		calls:   &calls,
		listErr: failure,
	}

	coordinator := NewRegistrationCoordinator(
		journal,
		nil,
		nil,
	)

	err := coordinator.RecoverPending(t.Context())

	if !errors.Is(err, failure) {
		t.Fatalf("expected journal error, got %v", err)
	}
}

type failingRecoveryAbortJournal struct {
	RegistrationJournal
	err error
}

func (j *failingRecoveryAbortJournal) DecideAbort(
	_ context.Context,
	_ uuid.UUID,
) error {
	return j.err
}

func TestRecoverPendingAbortDecisionFailure(t *testing.T) {
	failure := errors.New("database unavailable")

	var calls []string

	transactionID := uuid.New()

	baseJournal := &coordinatorJournalMock{
		calls: &calls,
		state: RegistrationPreparing,
		pending: []RegistrationTransaction{
			{
				TransactionID: transactionID,
				UserID:        uuid.New(),
				State:         RegistrationPreparing,
			},
		},
	}

	journal := &failingRecoveryAbortJournal{
		RegistrationJournal: baseJournal,
		err:                 failure,
	}

	auth := &coordinatorAuthMock{
		coordinatorBranchMock: &coordinatorBranchMock{
			name:  "auth",
			calls: &calls,
		},
	}

	identity := &coordinatorIdentityMock{
		coordinatorBranchMock: &coordinatorBranchMock{
			name:  "identity",
			calls: &calls,
		},
	}

	coordinator := NewRegistrationCoordinator(
		journal,
		auth,
		identity,
	)

	err := coordinator.RecoverPending(t.Context())

	if !errors.Is(err, failure) {
		t.Fatalf("expected journal failure, got %v", err)
	}

	if !errors.Is(err, ErrRegistrationPending) {
		t.Fatalf("expected pending transaction, got %v", err)
	}

	if baseJournal.state != RegistrationPreparing {
		t.Errorf(
			"state: got %s, want %s",
			baseJournal.state,
			RegistrationPreparing,
		)
	}

	if len(calls) != 0 {
		t.Errorf("unexpected participant calls: %v", calls)
	}
}

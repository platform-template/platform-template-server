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
		authCommitErr       error
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
				"lease.acquire",
				"identity.commit",
				"auth.commit",
				"journal.mark_committed",
				"lease.release",
			},
		},
		{
			name:              "identity commit still fails",
			state:             RegistrationCommitDecided,
			identityCommitErr: failure,
			wantState:         RegistrationCommitDecided,
			wantErr:           true,
			wantCalls: []string{
				"lease.acquire",
				"identity.commit",
				"lease.release",
			},
		},
		{
			name:          "auth commit still fails after identity commit",
			state:         RegistrationCommitDecided,
			authCommitErr: failure,
			wantState:     RegistrationCommitDecided,
			wantErr:       true,
			wantCalls: []string{
				"lease.acquire",
				"identity.commit",
				"auth.commit",
				"lease.release",
			},
		},
		{
			name:      "recover preparing transaction",
			state:     RegistrationPreparing,
			wantState: RegistrationAborted,
			wantCalls: []string{
				"lease.acquire",
				"journal.decide_abort",
				"auth.rollback",
				"identity.rollback",
				"journal.mark_aborted",
				"lease.release",
			},
		},
		{
			name:      "recover aborted decision",
			state:     RegistrationAbortDecided,
			wantState: RegistrationAborted,
			wantCalls: []string{
				"lease.acquire",
				"auth.rollback",
				"identity.rollback",
				"journal.mark_aborted",
				"lease.release",
			},
		},
		{
			name:                "identity rollback failed",
			state:               RegistrationAbortDecided,
			identityRollbackErr: failure,
			wantState:           RegistrationAbortDecided,
			wantErr:             true,
			wantCalls: []string{
				"lease.acquire",
				"auth.rollback",
				"identity.rollback",
				"lease.release",
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
					commitErr:   tt.authCommitErr,
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

			leases := &coordinatorLeaseMock{
				journal:        journal,
				calls:          &calls,
				acquireAllowed: true,
			}

			coordinator := NewRegistrationCoordinator(
				journal,
				leases,
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
		nil,
	)

	err := coordinator.RecoverPending(t.Context())

	if !errors.Is(err, failure) {
		t.Fatalf("expected journal error, got %v", err)
	}
}

type failingRecoveryAbortJournal struct {
	RegistrationJournal
	err   error
	calls *[]string
}

func (j *failingRecoveryAbortJournal) DecideAbort(
	_ context.Context,
	_ RegistrationLease,
) error {
	*j.calls = append(*j.calls, "journal.decide_abort")
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
		calls:               &calls,
	}

	leases := &coordinatorLeaseMock{
		journal:        baseJournal,
		calls:          &calls,
		acquireAllowed: true,
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
		leases,
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

	wantCalls := []string{
		"lease.acquire",
		"journal.decide_abort",
		"lease.release",
	}

	if !slices.Equal(calls, wantCalls) {
		t.Errorf(
			"calls: got %v, want %v",
			calls,
			wantCalls,
		)
	}
}

func TestRecoverPendingSkipsActiveTransaction(t *testing.T) {
	var calls []string

	transactionID := uuid.New()

	journal := &coordinatorJournalMock{
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

	leases := &coordinatorLeaseMock{
		journal:        journal,
		calls:          &calls,
		acquireAllowed: false,
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
		leases,
		auth,
		identity,
	)

	err := coordinator.RecoverPending(t.Context())
	if err != nil {
		t.Fatalf("unexpected recovery error: %v", err)
	}

	if journal.state != RegistrationPreparing {
		t.Errorf(
			"state: got %s, want %s",
			journal.state,
			RegistrationPreparing,
		)
	}

	wantCalls := []string{"lease.acquire"}

	if !slices.Equal(calls, wantCalls) {
		t.Errorf(
			"calls: got %v, want %v",
			calls,
			wantCalls,
		)
	}
}

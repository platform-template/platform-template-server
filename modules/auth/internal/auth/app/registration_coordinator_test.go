package app

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
	"uuid"
)

type coordinatorJournalMock struct {
	state           RegistrationState
	calls           *[]string
	decideCommitErr error
	decideAbortErr  error
	pending         []RegistrationTransaction
	listErr         error
}

func (m *coordinatorJournalMock) record(action string) {
	*m.calls = append(*m.calls, "journal."+action)
}

func (m *coordinatorJournalMock) DecideCommit(
	_ context.Context, _ RegistrationLease,
) error {
	m.record("decide_commit")
	if m.decideCommitErr != nil {
		return m.decideCommitErr
	}
	m.state = RegistrationCommitDecided
	return nil
}

func (m *coordinatorJournalMock) DecideAbort(
	_ context.Context, _ RegistrationLease,
) error {
	m.record("decide_abort")
	if m.decideAbortErr != nil {
		return m.decideAbortErr
	}
	m.state = RegistrationAbortDecided
	return nil
}

func (m *coordinatorJournalMock) MarkCommitted(
	_ context.Context, _ RegistrationLease,
) error {
	m.record("mark_committed")
	m.state = RegistrationCommitted
	return nil
}

func (m *coordinatorJournalMock) MarkAborted(
	_ context.Context, _ RegistrationLease,
) error {
	m.record("mark_aborted")
	m.state = RegistrationAborted
	return nil
}

func (m *coordinatorJournalMock) Get(
	_ context.Context, _ uuid.UUID,
) (RegistrationTransaction, error) {
	return RegistrationTransaction{State: m.state}, nil
}

func (f *coordinatorJournalMock) ListPending(
	_ context.Context,
) ([]RegistrationTransaction, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}

	return f.pending, nil
}

type coordinatorLeaseMock struct {
	journal        *coordinatorJournalMock
	calls          *[]string
	acquireAllowed bool
}

func (m *coordinatorLeaseMock) record(action string) {
	*m.calls = append(*m.calls, "lease."+action)
}

func (m *coordinatorLeaseMock) Begin(
	_ context.Context,
	transactionID, _ uuid.UUID,
	_ time.Duration,
) (RegistrationLease, error) {
	m.record("begin")
	m.journal.state = RegistrationPreparing

	return RegistrationLease{
		TransactionID: transactionID,
		OwnerID:       uuid.New(),
		Epoch:         1,
	}, nil
}

func (m *coordinatorLeaseMock) Acquire(
	_ context.Context,
	transactionID uuid.UUID,
	_ time.Duration,
) (RegistrationLease, bool, error) {
	m.record("acquire")

	if !m.acquireAllowed {
		return RegistrationLease{}, false, nil
	}

	return RegistrationLease{
		TransactionID: transactionID,
		OwnerID:       uuid.New(),
		Epoch:         2,
	}, true, nil
}

func (m *coordinatorLeaseMock) Renew(
	_ context.Context,
	_ RegistrationLease,
	_ time.Duration,
) (bool, error) {
	return true, nil
}

func (m *coordinatorLeaseMock) Release(
	_ context.Context,
	_ RegistrationLease,
) (bool, error) {
	m.record("release")
	return true, nil
}

type coordinatorBranchMock struct {
	name        string
	calls       *[]string
	prepareErr  error
	commitErr   error
	rollbackErr error
}

func (m *coordinatorBranchMock) record(action string) {
	*m.calls = append(*m.calls, m.name+"."+action)
}

func (m *coordinatorBranchMock) prepare() error {
	m.record("prepare")
	return m.prepareErr
}

func (m *coordinatorBranchMock) Commit(
	_ context.Context, _ uuid.UUID,
) error {
	m.record("commit")
	return m.commitErr
}

func (m *coordinatorBranchMock) Rollback(
	_ context.Context, _ uuid.UUID,
) error {
	m.record("rollback")
	return m.rollbackErr
}

type coordinatorAuthMock struct {
	*coordinatorBranchMock
}

func (m *coordinatorAuthMock) Prepare(
	_ context.Context,
	_, _ uuid.UUID,
	_, _ string,
) error {
	return m.prepare()
}

type coordinatorIdentityMock struct {
	*coordinatorBranchMock
}

func (m *coordinatorIdentityMock) Prepare(
	_ context.Context,
	_, _ uuid.UUID,
	_, _, _, _, _ string,
) error {
	return m.prepare()
}

func TestRegistrationCoordinator(t *testing.T) {
	failure := errors.New("simulated failure")

	tests := []struct {
		name               string
		authPrepareErr     error
		identityPrepareErr error
		identityCommitErr  error
		authRollbackErr    error
		decideCommitErr    error
		decideAbortErr     error
		wantState          RegistrationState
		wantError          bool
		wantPending        bool
		wantCalls          []string
	}{
		{
			name:      "successful registration",
			wantState: RegistrationCommitted,
			wantCalls: []string{
				"lease.begin",
				"auth.prepare",
				"identity.prepare",
				"journal.decide_commit",
				"auth.commit",
				"identity.commit",
				"journal.mark_committed",
				"lease.release",
			},
		},
		{
			name:               "identity prepare failed",
			identityPrepareErr: failure,
			wantState:          RegistrationAborted,
			wantError:          true,
			wantCalls: []string{
				"lease.begin",
				"auth.prepare",
				"identity.prepare",
				"journal.decide_abort",
				"auth.rollback",
				"identity.rollback",
				"journal.mark_aborted",
				"lease.release",
			},
		},
		{
			name:              "identity commit failed",
			identityCommitErr: failure,
			wantState:         RegistrationCommitDecided,
			wantError:         true,
			wantPending:       true,
			wantCalls: []string{
				"lease.begin",
				"auth.prepare",
				"identity.prepare",
				"journal.decide_commit",
				"auth.commit",
				"identity.commit",
				"lease.release",
			},
		},
		{
			name:            "commit decision not persisted",
			decideCommitErr: failure,
			wantState:       RegistrationPreparing,
			wantError:       true,
			wantPending:     true,
			wantCalls: []string{
				"lease.begin",
				"auth.prepare",
				"identity.prepare",
				"journal.decide_commit",
				"lease.release",
			},
		},
		{
			name:               "abort decision not persisted",
			identityPrepareErr: failure,
			decideAbortErr:     failure,
			wantState:          RegistrationPreparing,
			wantError:          true,
			wantPending:        true,
			wantCalls: []string{
				"lease.begin",
				"auth.prepare",
				"identity.prepare",
				"journal.decide_abort",
				"lease.release",
			},
		},
		{
			name:               "rollback failed",
			identityPrepareErr: failure,
			authRollbackErr:    failure,
			wantState:          RegistrationAbortDecided,
			wantError:          true,
			wantPending:        true,
			wantCalls: []string{
				"lease.begin",
				"auth.prepare",
				"identity.prepare",
				"journal.decide_abort",
				"auth.rollback",
				"identity.rollback",
				"lease.release",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string

			journal := &coordinatorJournalMock{
				calls:           &calls,
				decideCommitErr: tt.decideCommitErr,
				decideAbortErr:  tt.decideAbortErr,
			}

			auth := &coordinatorAuthMock{
				coordinatorBranchMock: &coordinatorBranchMock{
					name:        "auth",
					calls:       &calls,
					prepareErr:  tt.authPrepareErr,
					rollbackErr: tt.authRollbackErr,
				},
			}

			identity := &coordinatorIdentityMock{
				coordinatorBranchMock: &coordinatorBranchMock{
					name:       "identity",
					calls:      &calls,
					prepareErr: tt.identityPrepareErr,
					commitErr:  tt.identityCommitErr,
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

			userID, err := coordinator.Register(
				t.Context(),
				RegisterInput{
					FirstName: "Alex",
					LastName:  "Smith",
					Login:     "alex",
					Email:     "alex@example.com",
					Password:  "secure-password",
				},
			)

			if (err != nil) != tt.wantError {
				t.Fatalf("unexpected error: %v", err)
			}

			if errors.Is(err, ErrRegistrationPending) != tt.wantPending {
				t.Errorf("unexpected pending status: %v", err)
			}

			if userID == uuid.Nil() {
				t.Error("expected a generated user ID")
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
					"calls:\ngot  %v\nwant %v",
					calls,
					tt.wantCalls,
				)
			}
		})
	}
}

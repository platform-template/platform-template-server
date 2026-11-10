package app

import (
	"context"
	"errors"
	"slices"
	"testing"
	"uuid"
)

type coordinatorJournalMock struct {
	state           RegistrationState
	calls           *[]string
	decideCommitErr error
	decideAbortErr  error
}

func (m *coordinatorJournalMock) record(action string) {
	*m.calls = append(*m.calls, "journal."+action)
}

func (m *coordinatorJournalMock) Begin(
	_ context.Context, _, _ uuid.UUID,
) error {
	m.record("begin")
	m.state = RegistrationPreparing
	return nil
}

func (m *coordinatorJournalMock) DecideCommit(
	_ context.Context, _ uuid.UUID,
) error {
	m.record("decide_commit")
	if m.decideCommitErr != nil {
		return m.decideCommitErr
	}
	m.state = RegistrationCommitDecided
	return nil
}

func (m *coordinatorJournalMock) DecideAbort(
	_ context.Context, _ uuid.UUID,
) error {
	m.record("decide_abort")
	if m.decideAbortErr != nil {
		return m.decideAbortErr
	}
	m.state = RegistrationAbortDecided
	return nil
}

func (m *coordinatorJournalMock) MarkCommitted(
	_ context.Context, _ uuid.UUID,
) error {
	m.record("mark_committed")
	m.state = RegistrationCommitted
	return nil
}

func (m *coordinatorJournalMock) MarkAborted(
	_ context.Context, _ uuid.UUID,
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

func (m *coordinatorJournalMock) ListPending(
	_ context.Context,
) ([]RegistrationTransaction, error) {
	return nil, nil
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
				"journal.begin",
				"auth.prepare",
				"identity.prepare",
				"journal.decide_commit",
				"auth.commit",
				"identity.commit",
				"journal.mark_committed",
			},
		},
		{
			name:               "identity prepare failed",
			identityPrepareErr: failure,
			wantState:          RegistrationAborted,
			wantError:          true,
			wantCalls: []string{
				"journal.begin",
				"auth.prepare",
				"identity.prepare",
				"journal.decide_abort",
				"auth.rollback",
				"identity.rollback",
				"journal.mark_aborted",
			},
		},
		{
			name:              "identity commit failed",
			identityCommitErr: failure,
			wantState:         RegistrationCommitDecided,
			wantError:         true,
			wantPending:       true,
			wantCalls: []string{
				"journal.begin",
				"auth.prepare",
				"identity.prepare",
				"journal.decide_commit",
				"auth.commit",
				"identity.commit",
			},
		},
		{
			name:            "commit decision not persisted",
			decideCommitErr: failure,
			wantState:       RegistrationPreparing,
			wantError:       true,
			wantPending:     true,
			wantCalls: []string{
				"journal.begin",
				"auth.prepare",
				"identity.prepare",
				"journal.decide_commit",
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
				"journal.begin",
				"auth.prepare",
				"identity.prepare",
				"journal.decide_abort",
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
				"journal.begin",
				"auth.prepare",
				"identity.prepare",
				"journal.decide_abort",
				"auth.rollback",
				"identity.rollback",
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

			coordinator := NewRegistrationCoordinator(
				journal,
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

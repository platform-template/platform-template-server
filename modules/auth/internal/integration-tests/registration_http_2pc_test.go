package integration_tests

import (
	"database/sql"
	identityapi "identity/api/server/internalapi"
	"net/http"
	"testing"
	"time"
	"uuid"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"auth/internal/auth/app"
)

type registrationRow struct {
	transactionID uuid.UUID
	userID        uuid.UUID
	state         app.RegistrationState
}

func registrationSnapshot(
	t *testing.T,
	db *sql.DB,
) map[string]registrationRow {
	t.Helper()

	rows, err := db.QueryContext(
		t.Context(),
		`SELECT transaction_id, user_id, state
		 FROM registration_transactions`,
	)
	require.NoError(t, err)
	defer rows.Close()

	result := make(map[string]registrationRow)

	for rows.Next() {
		var transactionIDString string
		var userIDString string
		var state string

		err := rows.Scan(
			&transactionIDString,
			&userIDString,
			&state,
		)
		require.NoError(t, err)

		transactionID, err := uuid.Parse(transactionIDString)
		require.NoError(t, err)

		userID, err := uuid.Parse(userIDString)
		require.NoError(t, err)

		result[transactionIDString] = registrationRow{
			transactionID: transactionID,
			userID:        userID,
			state:         app.RegistrationState(state),
		}
	}

	require.NoError(t, rows.Err())
	return result
}

func newRegistrationSince(
	t *testing.T,
	db *sql.DB,
	before map[string]registrationRow,
) registrationRow {
	t.Helper()

	after := registrationSnapshot(t, db)
	var created []registrationRow

	for id, tx := range after {
		if _, exists := before[id]; !exists {
			created = append(created, tx)
		}
	}

	require.Len(t, created, 1)
	return created[0]
}

func freshRegistrationUser() testUser {
	suffix := uuid.New().String()

	return testUser{
		firstName: "Test",
		lastName:  "Registration",
		login:     "xa-" + suffix,
		email:     "xa-" + suffix + "@example.com",
		password:  initialPassword,
	}
}

func TestAuthHTTPRegistrationIdentityPrepareFailure(t *testing.T) {
	_, db := newXARegistrationParticipant(t)

	faults := testEnv.remotes.registrationFaults
	t.Cleanup(faults.reset)

	user := freshRegistrationUser()
	before := registrationSnapshot(t, db)

	faults.failNextPrepare()

	response := authRequest(
		t,
		authAPIPath+"/register",
		registerRequestFromUser(user),
		nil,
	)

	require.Equal(t, http.StatusInternalServerError, response.statusCode)

	tx := newRegistrationSince(t, db, before)
	require.Equal(t, app.RegistrationAborted, tx.state)

	require.Zero(t, countCredentials(
		t, t.Context(), db, tx.userID,
	))

	_, err := testEnv.remotes.identity.FindUserByLogin(
		t.Context(),
		&identityapi.FindUserByLoginRequest{
			Login: user.login,
		},
	)
	require.Equal(t, codes.NotFound, status.Code(err))
}

func TestAuthHTTPRegistrationAuthPrepareFailure(t *testing.T) {
	_, db := newXARegistrationParticipant(t)

	user := freshRegistrationUser()
	originalID := registerUser(t, user)

	before := registrationSnapshot(t, db)

	faults := testEnv.remotes.registrationFaults
	prepareCallsBefore := faults.prepareCount()

	duplicate := user
	duplicate.email = "duplicate-" + uuid.New().String() + "@example.com"

	response := authRequest(
		t,
		authAPIPath+"/register",
		registerRequestFromUser(duplicate),
		nil,
	)

	require.Equal(t, http.StatusConflict, response.statusCode)

	tx := newRegistrationSince(t, db, before)
	require.Equal(t, app.RegistrationAborted, tx.state)

	require.Zero(t, countCredentials(
		t, t.Context(), db, tx.userID,
	))

	require.Equal(
		t,
		prepareCallsBefore,
		faults.prepareCount(),
	)

	identityResponse, err := testEnv.remotes.identity.FindUserByLogin(
		t.Context(),
		&identityapi.FindUserByLoginRequest{
			Login: user.login,
		},
	)
	require.NoError(t, err)
	require.Equal(t, originalID, identityResponse.GetUserId())

	originalUUID, err := uuid.Parse(originalID)
	require.NoError(t, err)

	require.Equal(t, 1, countCredentials(
		t, t.Context(), db, originalUUID,
	))
}

func TestAuthHTTPRegistrationRecoveryAfterCommitFailure(t *testing.T) {
	_, db := newXARegistrationParticipant(t)

	faults := testEnv.remotes.registrationFaults
	t.Cleanup(faults.reset)

	user := freshRegistrationUser()
	before := registrationSnapshot(t, db)

	faults.failNextCommit()

	response := authRequest(
		t,
		authAPIPath+"/register",
		registerRequestFromUser(user),
		nil,
	)

	require.Equal(
		t,
		http.StatusServiceUnavailable,
		response.statusCode,
	)

	tx := newRegistrationSince(t, db, before)

	require.NotEqual(t, app.RegistrationAborted, tx.state)

	ctx := t.Context()

	require.Eventually(t, func() bool {
		var state string

		err := db.QueryRowContext(
			ctx,
			`SELECT state
			 FROM registration_transactions
			 WHERE transaction_id = ?`,
			tx.transactionID.String(),
		).Scan(&state)

		return err == nil &&
			state == string(app.RegistrationCommitted)
	}, 45*time.Second, 250*time.Millisecond)

	require.Equal(t, 1, countCredentials(
		t, ctx, db, tx.userID,
	))

	identityResponse, err := testEnv.remotes.identity.FindUserByLogin(
		ctx,
		&identityapi.FindUserByLoginRequest{
			Login: user.login,
		},
	)
	require.NoError(t, err)
	require.Equal(t, tx.userID.String(), identityResponse.GetUserId())

	_ = login(t, user, user.password)
}

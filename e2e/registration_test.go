//go:build e2e

package e2e

import (
	"context"
	"database/sql"
	"net/http"
	"testing"

	"e2e/internal/clients"
	identityapi "identity/api/server/internalapi"
	identityhttp "identity/api/server/publicapi"

	"github.com/stretchr/testify/require"
)

type registrationTransaction struct{ id, userID, state string }

func registrationTransactions(t *testing.T, ctx context.Context, db *sql.DB) map[string]registrationTransaction {
	t.Helper()
	rows, err := db.QueryContext(ctx, "SELECT transaction_id, user_id, state FROM registration_transactions")
	require.NoError(t, err)
	defer rows.Close()
	result := make(map[string]registrationTransaction)
	for rows.Next() {
		var tx registrationTransaction
		require.NoError(t, rows.Scan(&tx.id, &tx.userID, &tx.state))
		result[tx.id] = tx
	}
	require.NoError(t, rows.Err())
	return result
}

func newTransaction(t *testing.T, before, after map[string]registrationTransaction) registrationTransaction {
	t.Helper()
	var added []registrationTransaction
	for id, tx := range after {
		if _, ok := before[id]; !ok {
			added = append(added, tx)
		}
	}
	require.Len(t, added, 1)
	return added[0]
}

func countRows(t *testing.T, ctx context.Context, db *sql.DB, query string, args ...any) int {
	t.Helper()
	var count int
	require.NoError(t, db.QueryRowContext(ctx, query, args...).Scan(&count))
	return count
}

func assertNoPreparedTransaction(t *testing.T, ctx context.Context, db *sql.DB, transactionID string) {
	t.Helper()
	rows, err := db.QueryContext(ctx, "XA RECOVER")
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var formatID, gtridLength, bqualLength int
		var data []byte
		require.NoError(t, rows.Scan(&formatID, &gtridLength, &bqualLength, &data))
		require.LessOrEqual(t, gtridLength, len(data))
		require.NotEqual(t, transactionID, string(data[:gtridLength]))
	}
	require.NoError(t, rows.Err())
}

func TestRegisterUser(t *testing.T) {
	ctx, client, user := scenario(t)
	authDB, err := env.MySQL(ctx, "auth-mysql")
	require.NoError(t, err)
	identityDB, err := env.MySQL(ctx, "identity-mysql")
	require.NoError(t, err)

	response, err := client.RegisterWithResponse(ctx, user)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, response.StatusCode())
	require.NotNil(t, response.JSON201)
	id := response.JSON201.UserId.String()
	require.NotEqual(t, "00000000-0000-0000-0000-000000000000", id)

	identity, err := clients.Identity(ctx, env)
	require.NoError(t, err)
	found, err := identity.FindUserByLogin(ctx, &identityapi.FindUserByLoginRequest{Login: user.Login})
	require.NoError(t, err)
	require.Equal(t, id, found.GetUserId())
	address, err := env.HTTPAddress(ctx, "identity")
	require.NoError(t, err)
	profileClient, err := identityhttp.NewClientWithResponses(address)
	require.NoError(t, err)
	profile, err := profileClient.GetCurrentIdentityWithResponse(ctx, func(_ context.Context, req *http.Request) error {
		req.Header.Set("X-Auth-User", id)
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, profile.StatusCode())
	require.NotNil(t, profile.JSON200)
	require.Equal(t, id, profile.JSON200.UserId.String())
	require.Equal(t, user.Login, profile.JSON200.Login)
	require.Equal(t, user.FirstName, profile.JSON200.FirstName)
	require.Equal(t, user.LastName, profile.JSON200.LastName)
	require.Equal(t, user.MiddleName, profile.JSON200.MiddleName)
	require.Equal(t, user.Email, profile.JSON200.Email)

	var login, hash string
	require.NoError(t, authDB.QueryRowContext(
		ctx, "SELECT login, password_hash FROM password_credentials WHERE user_id = ?", id,
	).Scan(&login, &hash))
	require.Equal(t, user.Login, login)
	require.NotEmpty(t, hash)
	var txID, state string
	require.NoError(t, authDB.QueryRowContext(
		ctx, "SELECT transaction_id, state FROM registration_transactions WHERE user_id = ?", id,
	).Scan(&txID, &state))
	require.Equal(t, "COMMITTED", state)
	require.Equal(t, 1, countRows(
		t, ctx, authDB,
		"SELECT COUNT(*) FROM xa_branch_receipts WHERE transaction_id = ? AND branch = 'auth'", txID,
	))
	require.Equal(t, 1, countRows(
		t, ctx, identityDB,
		"SELECT COUNT(*) FROM xa_branch_receipts WHERE transaction_id = ? AND branch = 'identity'", txID,
	))
	authRoot, err := env.MySQLRoot(ctx, "auth-mysql")
	require.NoError(t, err)
	identityRoot, err := env.MySQLRoot(ctx, "identity-mysql")
	require.NoError(t, err)
	assertNoPreparedTransaction(t, ctx, authRoot, txID)
	assertNoPreparedTransaction(t, ctx, identityRoot, txID)
}

func TestRegisterDuplicateLogin(t *testing.T) {
	ctx, client, user := scenario(t)
	id := registeredUser(t, ctx, client, user)
	authDB, err := env.MySQL(ctx, "auth-mysql")
	require.NoError(t, err)
	identityDB, err := env.MySQL(ctx, "identity-mysql")
	require.NoError(t, err)
	before := registrationTransactions(t, ctx, authDB)
	duplicate := user
	duplicate.Email = "duplicate-" + user.Email

	response, err := client.RegisterWithResponse(ctx, duplicate)
	require.NoError(t, err)
	require.Equal(t, http.StatusConflict, response.StatusCode())

	tx := newTransaction(t, before, registrationTransactions(t, ctx, authDB))
	require.Equal(t, "ABORTED", tx.state)
	require.NotEqual(t, id, tx.userID)
	require.Equal(t, 1, countRows(
		t, ctx, authDB,
		"SELECT COUNT(*) FROM password_credentials WHERE login = ? AND user_id = ?", user.Login, id,
	))
	require.Equal(t, 1, countRows(
		t, ctx, authDB,
		"SELECT COUNT(*) FROM password_credentials WHERE login = ?", user.Login
	))
	require.Zero(t, countRows(
		t, ctx, authDB, 
		"SELECT COUNT(*) FROM password_credentials WHERE user_id = ?", tx.userID
	))
	require.Equal(t, 1, countRows(
		t, ctx, identityDB,
		"SELECT COUNT(*) FROM users WHERE login_snapshot = ? AND id = ?", user.Login, id,
	))
	require.Equal(t, 1, countRows(t, ctx, identityDB, "SELECT COUNT(*) FROM users WHERE login_snapshot = ?", user.Login))
	require.Zero(t, countRows(t, ctx, identityDB, "SELECT COUNT(*) FROM users WHERE id = ?", tx.userID))
	require.Zero(t, countRows(t, ctx, authDB, "SELECT COUNT(*) FROM xa_branch_receipts WHERE transaction_id = ?", tx.id))
	require.Zero(t, countRows(
		t, ctx, identityDB,
		"SELECT COUNT(*) FROM xa_branch_receipts WHERE transaction_id = ?", tx.id,
	))
	authRoot, err := env.MySQLRoot(ctx, "auth-mysql")
	require.NoError(t, err)
	identityRoot, err := env.MySQLRoot(ctx, "identity-mysql")
	require.NoError(t, err)
	assertNoPreparedTransaction(t, ctx, authRoot, tx.id)
	assertNoPreparedTransaction(t, ctx, identityRoot, tx.id)
}

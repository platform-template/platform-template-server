package xa

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var (
	ErrBranchAborted   = errors.New("xa: branch has been aborted")
	ErrBranchCommitted = errors.New("xa: branch has been committed")
)

const AbortBarrierMigrationSQL = `
CREATE TABLE IF NOT EXISTS xa_abort_barriers (
    transaction_id CHAR(36) CHARACTER SET ascii
        COLLATE ascii_bin NOT NULL,
    branch VARCHAR(64) CHARACTER SET ascii
        COLLATE ascii_bin NOT NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    PRIMARY KEY (transaction_id, branch)
) ENGINE=InnoDB;
`

func branchLockName(xid XID) string {
	hash := sha256.Sum256([]byte(
		xid.TransactionID.String() + ":" + xid.Branch,
	))
	return fmt.Sprintf("%x", hash)
}

func acquireBranchLock(
	ctx context.Context,
	conn *sql.Conn,
	xid XID,
) (func() error, error) {
	name := branchLockName(xid)

	var acquired sql.NullInt64
	err := conn.QueryRowContext(
		ctx,
		"SELECT GET_LOCK(?, 30)",
		name,
	).Scan(&acquired)
	if err != nil {
		discardConnection(conn)
		return nil, fmt.Errorf("xa: acquire branch lock: %w", err)
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		return nil, errors.New("xa: branch lock not acquired")
	}

	unlock := func() error {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			5*time.Second,
		)
		defer cancel()

		var released sql.NullInt64
		err := conn.QueryRowContext(
			ctx,
			"SELECT RELEASE_LOCK(?)",
			name,
		).Scan(&released)

		if err != nil || !released.Valid || released.Int64 != 1 {
			discardConnection(conn)

			return fmt.Errorf(
				"xa: release branch lock: result=%v: %w",
				released,
				errOrUnknown(err),
			)
		}

		return nil
	}

	return unlock, nil
}

func errOrUnknown(err error) error {
	if err != nil {
		return err
	}
	return errors.New("unexpected lock result")
}

func isBranchAborted(
	ctx context.Context,
	conn *sql.Conn,
	xid XID,
) (bool, error) {
	var found int

	err := conn.QueryRowContext(ctx, `
SELECT 1
FROM xa_abort_barriers
WHERE transaction_id = ? AND branch = ?
`, xid.TransactionID.String(), xid.Branch).Scan(&found)

	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

func isBranchCommitted(
	ctx context.Context,
	conn *sql.Conn,
	xid XID,
) (bool, error) {
	var found int

	err := conn.QueryRowContext(ctx, `
SELECT 1
FROM xa_branch_receipts
WHERE transaction_id = ? AND branch = ?
`, xid.TransactionID.String(), xid.Branch).Scan(&found)

	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

func recordAbortBarrier(
	ctx context.Context,
	conn *sql.Conn,
	xid XID,
) error {
	_, err := conn.ExecContext(ctx, `
INSERT IGNORE INTO xa_abort_barriers (transaction_id, branch)
VALUES (?, ?)
`, xid.TransactionID.String(), xid.Branch)

	return err
}

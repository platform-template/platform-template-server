package xa

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const ReceiptMigrationSQL = `
CREATE TABLE IF NOT EXISTS xa_branch_receipts (
    transaction_id CHAR(36) CHARACTER SET ascii
        COLLATE ascii_bin NOT NULL,
    branch VARCHAR(64) CHARACTER SET ascii
        COLLATE ascii_bin NOT NULL,
    committed_at DATETIME(6) NOT NULL
        DEFAULT CURRENT_TIMESTAMP(6),

    PRIMARY KEY (transaction_id, branch)
) ENGINE=InnoDB;
`

func RecordCommitReceipt(
	ctx context.Context,
	executor Executor,
	xid XID,
) error {
	if err := xid.validate(); err != nil {
		return err
	}

	_, err := executor.ExecContext(ctx, `
INSERT INTO xa_branch_receipts (transaction_id, branch)
VALUES (?, ?)
`, xid.TransactionID.String(), xid.Branch)

	if err != nil {
		return fmt.Errorf("xa: record commit receipt: %w", err)
	}

	return nil
}

func (m *MySQL) Committed(
	ctx context.Context,
	xid XID,
) (bool, error) {
	if err := xid.validate(); err != nil {
		return false, err
	}

	var found int

	err := m.db.QueryRowContext(ctx, `
SELECT 1
FROM xa_branch_receipts
WHERE transaction_id = ? AND branch = ?
`, xid.TransactionID.String(), xid.Branch).Scan(&found)

	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("xa: check commit receipt: %w", err)
	}

	return true, nil
}

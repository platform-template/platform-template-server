package xa

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"time"
	"uuid"

	mysqldriver "github.com/go-sql-driver/mysql"
)

const cleanupTimeout = 5 * time.Second

var ErrOutcomeUnknown = errors.New("xa: transaction outcome unknown")

type Executor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

type Resource interface {
	Prepare(ctx context.Context, xid XID, operation func(context.Context, Executor) error) error
	Commit(ctx context.Context, xid XID) error
	Rollback(ctx context.Context, xid XID) error
	Recover(ctx context.Context) ([]XID, error)
}

type MySQL struct {
	db *sql.DB
}

var _ Resource = (*MySQL)(nil)

func NewMySQL(db *sql.DB) (*MySQL, error) {
	if db == nil {
		return nil, errors.New("xa: database must not be nil")
	}

	return &MySQL{db: db}, nil
}

func (m *MySQL) Prepare(
	ctx context.Context,
	xid XID,
	operation func(context.Context, Executor) error,
) (err error) {
	if err := xid.validate(); err != nil {
		return err
	}
	if operation == nil {
		return errors.New("xa: operation must not be nil")
	}

	conn, err := m.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("xa: acquire connection: %w", err)
	}
	defer func() {
		err = errors.Join(err, conn.Close())
	}()

	unlock, err := acquireBranchLock(ctx, conn, xid)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, unlock())
	}()

	aborted, err := isBranchAborted(ctx, conn, xid)
	if err != nil {
		return fmt.Errorf("xa: check abort barrier: %w", err)
	}
	if aborted {
		return ErrBranchAborted
	}

	sqlXID := xid.sqlLiteral()

	if _, err := conn.ExecContext(ctx, "XA START "+sqlXID); err != nil {
		discardConnection(conn)
		return fmt.Errorf("xa: start: %w", err)
	}

	if err := operation(ctx, conn); err != nil {
		rollbackErr := rollbackActive(conn, sqlXID)
		if rollbackErr != nil {
			discardConnection(conn)
		}
		return errors.Join(fmt.Errorf("xa: operation: %w", err), rollbackErr)
	}

	if _, err := conn.ExecContext(ctx, "XA END "+sqlXID); err != nil {
		discardConnection(conn)
		return fmt.Errorf("%w: XA END: %w", ErrOutcomeUnknown, err)
	}

	if _, err := conn.ExecContext(ctx, "XA PREPARE "+sqlXID); err != nil {
		discardConnection(conn)
		return fmt.Errorf("%w: XA PREPARE: %w", ErrOutcomeUnknown, err)
	}

	return nil
}

func (m *MySQL) Commit(ctx context.Context, xid XID) (err error) {
	if err := xid.validate(); err != nil {
		return err
	}

	conn, err := m.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("xa: acquire connection: %w", err)
	}
	defer func() {
		err = errors.Join(err, conn.Close())
	}()

	unlock, err := acquireBranchLock(ctx, conn, xid)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, unlock())
	}()

	aborted, err := isBranchAborted(ctx, conn, xid)
	if err != nil {
		return fmt.Errorf("xa: check abort barrier: %w", err)
	}
	if aborted {
		return ErrBranchAborted
	}

	_, commitErr := conn.ExecContext(
		ctx,
		"XA COMMIT "+xid.sqlLiteral(),
	)
	if commitErr == nil {
		return nil
	}

	committed, checkErr := isBranchCommitted(ctx, conn, xid)
	if checkErr != nil {
		return errors.Join(
			fmt.Errorf("%w: XA COMMIT: %w", ErrOutcomeUnknown, commitErr),
			fmt.Errorf("xa: verify commit receipt: %w", checkErr),
		)
	}

	if committed {
		return nil
	}

	return fmt.Errorf(
		"%w: XA COMMIT: %w",
		ErrOutcomeUnknown,
		commitErr,
	)
}

func (m *MySQL) Rollback(ctx context.Context, xid XID) (err error) {
	if err := xid.validate(); err != nil {
		return err
	}

	conn, err := m.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("xa: acquire connection: %w", err)
	}
	defer func() {
		err = errors.Join(err, conn.Close())
	}()

	unlock, err := acquireBranchLock(ctx, conn, xid)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, unlock())
	}()

	committed, err := isBranchCommitted(ctx, conn, xid)
	if err != nil {
		return fmt.Errorf("xa: check commit receipt: %w", err)
	}
	if committed {
		return ErrBranchCommitted
	}

	if err := recordAbortBarrier(ctx, conn, xid); err != nil {
		return fmt.Errorf("xa: record abort barrier: %w", err)
	}

	_, rollbackErr := conn.ExecContext(
		ctx,
		"XA ROLLBACK "+xid.sqlLiteral(),
	)
	if rollbackErr == nil {
		return nil
	}

	var mysqlErr *mysqldriver.MySQLError
	if errors.As(rollbackErr, &mysqlErr) && mysqlErr.Number == 1397 {
		return nil
	}

	return fmt.Errorf(
		"%w: XA ROLLBACK: %w",
		ErrOutcomeUnknown,
		rollbackErr,
	)
}

func (m *MySQL) Recover(ctx context.Context) ([]XID, error) {
	rows, err := m.db.QueryContext(ctx, "XA RECOVER")
	if err != nil {
		return nil, fmt.Errorf("xa: recover: %w", err)
	}
	defer rows.Close()

	var result []XID

	for rows.Next() {
		var recoveredFormat, globalLength, branchLength int
		var data []byte

		if err := rows.Scan(&recoveredFormat, &globalLength, &branchLength, &data); err != nil {
			return nil, fmt.Errorf("xa: scan recovered transaction: %w", err)
		}

		if recoveredFormat != formatID {
			continue
		}

		if globalLength < 0 || branchLength < 0 || globalLength+branchLength != len(data) {
			return nil, errors.New("xa: invalid recovered transaction data")
		}

		txID, err := uuid.Parse(string(data[:globalLength]))
		if err != nil {
			return nil, fmt.Errorf("xa: parse recovered transaction ID: %w", err)
		}

		xid := XID{
			TransactionID: txID,
			Branch:        string(data[globalLength:]),
		}
		if err := xid.validate(); err != nil {
			return nil, fmt.Errorf("xa: validate recovered transaction: %w", err)
		}

		result = append(result, xid)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("xa: iterate recovered transactions: %w", err)
	}

	return result, nil
}

func rollbackActive(conn *sql.Conn, sqlXID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), cleanupTimeout)
	defer cancel()

	if _, err := conn.ExecContext(ctx, "XA END "+sqlXID); err != nil {
		return fmt.Errorf("xa: cleanup end: %w", err)
	}

	if _, err := conn.ExecContext(ctx, "XA ROLLBACK "+sqlXID); err != nil {
		return fmt.Errorf("xa: cleanup rollback: %w", err)
	}

	return nil
}

func discardConnection(conn *sql.Conn) {
	_ = conn.Raw(func(any) error {
		return driver.ErrBadConn
	})
}

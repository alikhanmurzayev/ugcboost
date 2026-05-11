package service

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// testTx satisfies pgx.Tx for unit-testing dbutil.WithTx.
// Commit/Rollback are no-ops; other methods panic because mock repos
// intercept all DB operations before they reach the underlying connection.
type testTx struct{}

func (testTx) Commit(context.Context) error   { return nil }
func (testTx) Rollback(context.Context) error { return nil }

// recordingTx is the same shape as testTx but records whether Commit /
// Rollback fired. Use it when a test needs to verify dbutil.WithTx really
// rolled back (e.g. callback returned an error and the data layer must not
// commit the partial write).
type recordingTx struct {
	testTx
	committed  bool
	rolledBack bool
}

func (t *recordingTx) Commit(context.Context) error {
	t.committed = true
	return nil
}

func (t *recordingTx) Rollback(context.Context) error {
	t.rolledBack = true
	return nil
}
func (testTx) Begin(context.Context) (pgx.Tx, error) {
	panic("testTx: unexpected Begin")
}
func (testTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	panic("testTx: unexpected CopyFrom")
}
func (testTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults {
	panic("testTx: unexpected SendBatch")
}
func (testTx) LargeObjects() pgx.LargeObjects {
	panic("testTx: unexpected LargeObjects")
}
func (testTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	panic("testTx: unexpected Prepare")
}
func (testTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	panic("testTx: unexpected Exec")
}
func (testTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("testTx: unexpected Query")
}
func (testTx) QueryRow(context.Context, string, ...any) pgx.Row {
	panic("testTx: unexpected QueryRow")
}
func (testTx) Conn() *pgx.Conn { panic("testTx: unexpected Conn") }

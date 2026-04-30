package dbutil_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
)

// stubTx is a pgx.Tx that records Commit/Rollback calls and lets each test
// override individual outcomes. All Query/Exec/etc. methods panic — none
// of these tests reach them.
type stubTx struct {
	commits        int
	rollbacks      int
	rollbackErr    error
	rollbackCtxErr error // ctx.Err() captured at the time of the Rollback call
}

func (t *stubTx) Begin(context.Context) (pgx.Tx, error) {
	panic("stubTx: unexpected Begin")
}
func (t *stubTx) Commit(context.Context) error { t.commits++; return nil }
func (t *stubTx) Rollback(ctx context.Context) error {
	t.rollbacks++
	t.rollbackCtxErr = ctx.Err()
	return t.rollbackErr
}
func (t *stubTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	panic("stubTx: unexpected CopyFrom")
}
func (t *stubTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults {
	panic("stubTx: unexpected SendBatch")
}
func (t *stubTx) LargeObjects() pgx.LargeObjects { panic("stubTx: unexpected LargeObjects") }
func (t *stubTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	panic("stubTx: unexpected Prepare")
}
func (t *stubTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	panic("stubTx: unexpected Exec")
}
func (t *stubTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("stubTx: unexpected Query")
}
func (t *stubTx) QueryRow(context.Context, string, ...any) pgx.Row {
	panic("stubTx: unexpected QueryRow")
}
func (t *stubTx) Conn() *pgx.Conn { panic("stubTx: unexpected Conn") }

type stubStarter struct {
	tx       *stubTx
	beginErr error
}

func (s *stubStarter) Begin(context.Context) (pgx.Tx, error) {
	if s.beginErr != nil {
		return nil, s.beginErr
	}
	return s.tx, nil
}

func TestWithTx_Panic(t *testing.T) {
	t.Parallel()

	t.Run("panic in fn → rollback + re-panic", func(t *testing.T) {
		t.Parallel()
		tx := &stubTx{}
		starter := &stubStarter{tx: tx}

		require.PanicsWithValue(t, "boom", func() {
			_ = dbutil.WithTx(context.Background(), starter, func(dbutil.DB) error {
				panic("boom")
			})
		})
		require.Equal(t, 1, tx.rollbacks, "rollback must be called on panic")
		require.Equal(t, 0, tx.commits, "commit must NOT be called on panic")
	})

	t.Run("panic + rollback error → still re-panics, rollback error swallowed", func(t *testing.T) {
		t.Parallel()
		tx := &stubTx{rollbackErr: errors.New("rollback boom")}
		starter := &stubStarter{tx: tx}

		require.PanicsWithValue(t, "boom", func() {
			_ = dbutil.WithTx(context.Background(), starter, func(dbutil.DB) error {
				panic("boom")
			})
		})
		require.Equal(t, 1, tx.rollbacks)
	})

	t.Run("happy path → commit, no rollback, no panic", func(t *testing.T) {
		t.Parallel()
		tx := &stubTx{}
		starter := &stubStarter{tx: tx}

		err := dbutil.WithTx(context.Background(), starter, func(dbutil.DB) error {
			return nil
		})
		require.NoError(t, err)
		require.Equal(t, 1, tx.commits)
		require.Equal(t, 0, tx.rollbacks)
	})

	t.Run("fn returns error → rollback, no panic, error propagated", func(t *testing.T) {
		t.Parallel()
		tx := &stubTx{}
		starter := &stubStarter{tx: tx}
		want := errors.New("biz error")

		err := dbutil.WithTx(context.Background(), starter, func(dbutil.DB) error {
			return want
		})
		require.ErrorIs(t, err, want)
		require.Equal(t, 1, tx.rollbacks)
		require.Equal(t, 0, tx.commits)
	})

	t.Run("rollback uses context.WithoutCancel even if outer ctx cancelled", func(t *testing.T) {
		t.Parallel()
		tx := &stubTx{}
		starter := &stubStarter{tx: tx}

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // pre-cancel before WithTx is called

		err := dbutil.WithTx(ctx, starter, func(dbutil.DB) error {
			return errors.New("biz")
		})
		require.Error(t, err)
		require.Equal(t, 1, tx.rollbacks)
		require.NoError(t, tx.rollbackCtxErr, "rollback ctx must NOT be cancelled (WithoutCancel)")
	})

	t.Run("panic + cancelled ctx → rollback still goes on the wire", func(t *testing.T) {
		t.Parallel()
		tx := &stubTx{}
		starter := &stubStarter{tx: tx}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		require.PanicsWithValue(t, "boom", func() {
			_ = dbutil.WithTx(ctx, starter, func(dbutil.DB) error {
				panic("boom")
			})
		})
		require.Equal(t, 1, tx.rollbacks)
		require.NoError(t, tx.rollbackCtxErr, "rollback ctx on panic-path must NOT be cancelled")
	})
}

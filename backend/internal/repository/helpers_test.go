package repository

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/mock"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil/mocks"
)

// captureQuery sets up a mock DB.Query expectation that captures SQL and args.
// Returns an error to prevent pgx row scanning (success path is covered by E2E).
func captureQuery(t *testing.T, db *mocks.MockDB, numQueryArgs int) (sql *string, args *[]any) {
	t.Helper()
	var capturedSQL string
	var capturedArgs []any

	matchers := []interface{}{mock.Anything, mock.Anything}
	if numQueryArgs > 0 {
		matchers = append(matchers, mock.Anything)
	}

	db.On("Query", matchers...).
		Run(func(callArgs mock.Arguments) {
			capturedSQL = callArgs.String(1)
			if len(callArgs) > 2 {
				capturedArgs = callArgs[2].([]any)
			}
		}).
		Return(nil, errors.New("mock: query intercepted")).
		Once()

	return &capturedSQL, &capturedArgs
}

// captureExec sets up a mock DB.Exec expectation that captures SQL and args.
// Returns a success CommandTag.
func captureExec(t *testing.T, db *mocks.MockDB, numExecArgs int) (sql *string, args *[]any) {
	t.Helper()
	var capturedSQL string
	var capturedArgs []any

	matchers := []interface{}{mock.Anything, mock.Anything}
	if numExecArgs > 0 {
		matchers = append(matchers, mock.Anything)
	}

	db.On("Exec", matchers...).
		Run(func(callArgs mock.Arguments) {
			capturedSQL = callArgs.String(1)
			if len(callArgs) > 2 {
				capturedArgs = callArgs[2].([]any)
			}
		}).
		Return(pgconn.NewCommandTag("OK"), nil).
		Once()

	return &capturedSQL, &capturedArgs
}

// scalarRows is a minimal pgx.Rows implementation that returns a single int64 value.
// Used for testing methods with two SQL queries (COUNT + SELECT), where the first
// query must succeed for the method to reach the second one.
type scalarRows struct {
	val    int64
	called bool
}

var _ pgx.Rows = (*scalarRows)(nil)

func (r *scalarRows) Close()                                       {}
func (r *scalarRows) Err() error                                   { return nil }
func (r *scalarRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (r *scalarRows) FieldDescriptions() []pgconn.FieldDescription { return nil }
func (r *scalarRows) Values() ([]any, error)                       { return nil, nil }
func (r *scalarRows) RawValues() [][]byte                          { return nil }
func (r *scalarRows) Conn() *pgx.Conn                             { return nil }

func (r *scalarRows) Next() bool {
	if !r.called {
		r.called = true
		return true
	}
	return false
}

func (r *scalarRows) Scan(dest ...any) error {
	*dest[0].(*int64) = r.val
	return nil
}

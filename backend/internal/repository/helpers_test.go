package repository

import (
	"testing"

	"github.com/pashagolub/pgxmock/v4"
)

// newPgxmock creates a pgxmock pool wired to satisfy dbutil.DB with strict
// SQL equality matching (QueryMatcherEqual). It registers a cleanup that asserts
// all queued expectations were met and closes the pool. Tests create one per
// t.Run for isolation.
func newPgxmock(t *testing.T) pgxmock.PgxPoolIface {
	t.Helper()
	mock, err := pgxmock.NewPool(pgxmock.QueryMatcherOption(pgxmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("pgxmock.NewPool: %v", err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet pgxmock expectations: %v", err)
		}
		mock.Close()
	})
	return mock
}

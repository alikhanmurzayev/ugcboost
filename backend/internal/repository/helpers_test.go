package repository

import (
	"testing"

	"github.com/elgris/stom"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"
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

func TestToMap_PanicsOnUnmappableValue(t *testing.T) {
	t.Parallel()
	// stom.ToMap requires a struct (or pointer to one). Passing a non-struct
	// triggers an error which toMap intentionally upgrades to a panic — the
	// repo layer is the only caller and a misuse there is a programming bug,
	// not a runtime failure mode.
	st := stom.MustNewStom(BrandRow{}).SetTag(string(tagInsert))
	require.Panics(t, func() {
		toMap(42, st)
	})
}

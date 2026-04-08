package repository

import (
	"github.com/alikhanmurzayev/ugcboost/backend/internal/dbutil"
)

// RepoFactory provides access to all repositories, sharing the same DB connection.
// Pass pgxpool.Pool for normal operations or pgx.Tx (via dbutil.WithTx) for transactions.
type RepoFactory struct {
	db dbutil.DB
}

// NewRepoFactory creates a new repository factory.
func NewRepoFactory(db dbutil.DB) *RepoFactory {
	return &RepoFactory{db: db}
}

// DB returns the underlying database connection.
func (f *RepoFactory) DB() dbutil.DB {
	return f.db
}

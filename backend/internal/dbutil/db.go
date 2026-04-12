package dbutil

import (
	"context"
	"errors"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DB is the common interface implemented by both pgxpool.Pool and pgx.Tx.
// Repositories accept DB so they work transparently inside or outside a transaction.
type DB interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// Psql is the PostgreSQL-flavoured squirrel statement builder ($1, $2, ...).
var Psql = sq.StatementBuilder.PlaceholderFormat(sq.Dollar)

// One executes a squirrel query and scans a single row into *T.
// Returns pgx.ErrNoRows (wrapped) when no row matches.
func One[T any](ctx context.Context, db DB, query sq.Sqlizer) (*T, error) {
	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("dbutil.One build sql: %w", err)
	}
	rows, err := db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("dbutil.One query: %w", err)
	}
	result, err := pgx.CollectOneRow(rows, pgx.RowToStructByName[T])
	if err != nil {
		return nil, fmt.Errorf("dbutil.One scan: %w", err)
	}
	return &result, nil
}

// Many executes a squirrel query and scans all rows into []*T.
func Many[T any](ctx context.Context, db DB, query sq.Sqlizer) ([]*T, error) {
	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("dbutil.Many build sql: %w", err)
	}
	rows, err := db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("dbutil.Many query: %w", err)
	}
	results, err := pgx.CollectRows(rows, pgx.RowToStructByName[T])
	if err != nil {
		return nil, fmt.Errorf("dbutil.Many scan: %w", err)
	}
	ptrs := make([]*T, len(results))
	for i := range results {
		ptrs[i] = &results[i]
	}
	return ptrs, nil
}

// Val executes a squirrel query and returns a single scalar value.
func Val[T any](ctx context.Context, db DB, query sq.Sqlizer) (T, error) {
	var zero T
	sql, args, err := query.ToSql()
	if err != nil {
		return zero, fmt.Errorf("dbutil.Val build sql: %w", err)
	}
	rows, err := db.Query(ctx, sql, args...)
	if err != nil {
		return zero, fmt.Errorf("dbutil.Val query: %w", err)
	}
	result, err := pgx.CollectOneRow(rows, pgx.RowTo[T])
	if err != nil {
		return zero, fmt.Errorf("dbutil.Val scan: %w", err)
	}
	return result, nil
}

// Vals executes a squirrel query and returns a slice of scalar values.
func Vals[T any](ctx context.Context, db DB, query sq.Sqlizer) ([]T, error) {
	sql, args, err := query.ToSql()
	if err != nil {
		return nil, fmt.Errorf("dbutil.Vals build sql: %w", err)
	}
	rows, err := db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("dbutil.Vals query: %w", err)
	}
	result, err := pgx.CollectRows(rows, pgx.RowTo[T])
	if err != nil {
		return nil, fmt.Errorf("dbutil.Vals scan: %w", err)
	}
	return result, nil
}

// Exec executes a squirrel INSERT/UPDATE/DELETE and returns rows affected.
func Exec(ctx context.Context, db DB, query sq.Sqlizer) (int64, error) {
	sql, args, err := query.ToSql()
	if err != nil {
		return 0, fmt.Errorf("dbutil.Exec build sql: %w", err)
	}
	tag, err := db.Exec(ctx, sql, args...)
	if err != nil {
		return 0, fmt.Errorf("dbutil.Exec: %w", err)
	}
	return tag.RowsAffected(), nil
}

// TxStarter abstracts the ability to begin a transaction.
type TxStarter interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// WithTx runs fn inside a database transaction. If fn returns an error the
// transaction is rolled back; otherwise it is committed.
// Both fn error and rollback error are joined if both occur.
func WithTx(ctx context.Context, starter TxStarter, fn func(tx DB) error) error {
	tx, err := starter.Begin(ctx)
	if err != nil {
		return fmt.Errorf("dbutil.WithTx begin: %w", err)
	}

	if fnErr := fn(tx); fnErr != nil {
		if rbErr := tx.Rollback(ctx); rbErr != nil {
			return errors.Join(fnErr, fmt.Errorf("rollback: %w", rbErr))
		}
		return fnErr
	}
	return tx.Commit(ctx)
}

package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

func TestCampaignRepository_Create(t *testing.T) {
	t.Parallel()

	const sqlStmt = "INSERT INTO campaigns (name,tma_url) VALUES ($1,$2) RETURNING created_at, id, is_deleted, name, tma_url, updated_at"

	t.Run("success maps row to struct", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}
		createdAt := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("Promo X", "https://tma.ugcboost.kz/tz/abc").
			WillReturnRows(pgxmock.NewRows([]string{"created_at", "id", "is_deleted", "name", "tma_url", "updated_at"}).
				AddRow(createdAt, "c-1", false, "Promo X", "https://tma.ugcboost.kz/tz/abc", createdAt))

		got, err := repo.Create(context.Background(), "Promo X", "https://tma.ugcboost.kz/tz/abc")
		require.NoError(t, err)
		require.Equal(t, &CampaignRow{
			ID:        "c-1",
			Name:      "Promo X",
			TmaURL:    "https://tma.ugcboost.kz/tz/abc",
			IsDeleted: false,
			CreatedAt: createdAt,
			UpdatedAt: createdAt,
		}, got)
	})

	t.Run("name taken returns ErrCampaignNameTaken", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("Promo X", "https://tma.ugcboost.kz/tz/abc").
			WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: CampaignsNameActiveUnique})

		_, err := repo.Create(context.Background(), "Promo X", "https://tma.ugcboost.kz/tz/abc")
		require.ErrorIs(t, err, domain.ErrCampaignNameTaken)
	})

	t.Run("unrelated 23505 propagates raw", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}
		pgErr := &pgconn.PgError{Code: "23505", ConstraintName: "campaigns_other_unique"}

		mock.ExpectQuery(sqlStmt).
			WithArgs("Promo X", "https://tma.ugcboost.kz/tz/abc").
			WillReturnError(pgErr)

		_, err := repo.Create(context.Background(), "Promo X", "https://tma.ugcboost.kz/tz/abc")
		require.NotErrorIs(t, err, domain.ErrCampaignNameTaken)
		require.ErrorIs(t, err, pgErr)
	})

	t.Run("propagates other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("Promo X", "https://tma.ugcboost.kz/tz/abc").
			WillReturnError(errors.New("db unavailable"))

		_, err := repo.Create(context.Background(), "Promo X", "https://tma.ugcboost.kz/tz/abc")
		require.ErrorContains(t, err, "db unavailable")
	})
}

func TestCampaignRepository_GetByID(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT created_at, id, is_deleted, name, tma_url, updated_at FROM campaigns WHERE id = $1"

	t.Run("success maps row to struct", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}
		createdAt := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("c-1").
			WillReturnRows(pgxmock.NewRows([]string{"created_at", "id", "is_deleted", "name", "tma_url", "updated_at"}).
				AddRow(createdAt, "c-1", false, "Promo X", "https://tma.ugcboost.kz/tz/abc", createdAt))

		got, err := repo.GetByID(context.Background(), "c-1")
		require.NoError(t, err)
		require.Equal(t, &CampaignRow{
			ID:        "c-1",
			Name:      "Promo X",
			TmaURL:    "https://tma.ugcboost.kz/tz/abc",
			IsDeleted: false,
			CreatedAt: createdAt,
			UpdatedAt: createdAt,
		}, got)
	})

	t.Run("success returns soft-deleted row (no is_deleted filter)", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}
		createdAt := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("c-2").
			WillReturnRows(pgxmock.NewRows([]string{"created_at", "id", "is_deleted", "name", "tma_url", "updated_at"}).
				AddRow(createdAt, "c-2", true, "Promo Y", "https://tma.ugcboost.kz/tz/y", createdAt))

		got, err := repo.GetByID(context.Background(), "c-2")
		require.NoError(t, err)
		require.True(t, got.IsDeleted, "GetByID must return soft-deleted rows untouched — admin contract")
	})

	t.Run("not found propagates sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("missing").
			WillReturnError(sql.ErrNoRows)

		_, err := repo.GetByID(context.Background(), "missing")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("propagates other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("c-1").
			WillReturnError(errors.New("db unavailable"))

		_, err := repo.GetByID(context.Background(), "c-1")
		require.ErrorContains(t, err, "db unavailable")
	})
}

func TestCampaignRepository_ListByIDs(t *testing.T) {
	t.Parallel()

	t.Run("empty ids short-circuits without query", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}

		rows, err := repo.ListByIDs(context.Background(), nil)
		require.NoError(t, err)
		require.Nil(t, rows)
	})

	t.Run("success maps single row", func(t *testing.T) {
		t.Parallel()
		const sqlStmt = "SELECT created_at, id, is_deleted, name, tma_url, updated_at FROM campaigns WHERE id IN ($1)"
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}
		createdAt := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("c-1").
			WillReturnRows(pgxmock.NewRows([]string{"created_at", "id", "is_deleted", "name", "tma_url", "updated_at"}).
				AddRow(createdAt, "c-1", false, "Promo X", "https://tma.ugcboost.kz/tz/abc", createdAt))

		got, err := repo.ListByIDs(context.Background(), []string{"c-1"})
		require.NoError(t, err)
		require.Equal(t, []*CampaignRow{{
			ID:        "c-1",
			Name:      "Promo X",
			TmaURL:    "https://tma.ugcboost.kz/tz/abc",
			IsDeleted: false,
			CreatedAt: createdAt,
			UpdatedAt: createdAt,
		}}, got)
	})

	t.Run("success maps N rows", func(t *testing.T) {
		t.Parallel()
		const sqlStmt = "SELECT created_at, id, is_deleted, name, tma_url, updated_at FROM campaigns WHERE id IN ($1,$2,$3)"
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}
		createdAt := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("c-1", "c-2", "c-3").
			WillReturnRows(pgxmock.NewRows([]string{"created_at", "id", "is_deleted", "name", "tma_url", "updated_at"}).
				AddRow(createdAt, "c-1", false, "Promo A", "https://tma.ugcboost.kz/tz/a", createdAt).
				AddRow(createdAt, "c-2", true, "Promo B", "https://tma.ugcboost.kz/tz/b", createdAt).
				AddRow(createdAt, "c-3", false, "Promo C", "https://tma.ugcboost.kz/tz/c", createdAt))

		got, err := repo.ListByIDs(context.Background(), []string{"c-1", "c-2", "c-3"})
		require.NoError(t, err)
		require.Equal(t, []*CampaignRow{
			{ID: "c-1", Name: "Promo A", TmaURL: "https://tma.ugcboost.kz/tz/a", IsDeleted: false, CreatedAt: createdAt, UpdatedAt: createdAt},
			{ID: "c-2", Name: "Promo B", TmaURL: "https://tma.ugcboost.kz/tz/b", IsDeleted: true, CreatedAt: createdAt, UpdatedAt: createdAt},
			{ID: "c-3", Name: "Promo C", TmaURL: "https://tma.ugcboost.kz/tz/c", IsDeleted: false, CreatedAt: createdAt, UpdatedAt: createdAt},
		}, got)
	})

	t.Run("propagates query error wrapped with method context", func(t *testing.T) {
		t.Parallel()
		const sqlStmt = "SELECT created_at, id, is_deleted, name, tma_url, updated_at FROM campaigns WHERE id IN ($1,$2)"
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("c-1", "c-2").
			WillReturnError(errors.New("db down"))

		_, err := repo.ListByIDs(context.Background(), []string{"c-1", "c-2"})
		require.ErrorContains(t, err, "campaign_repository.ListByIDs:")
		require.ErrorContains(t, err, "db down")
	})
}

func TestCampaignRepository_Update(t *testing.T) {
	t.Parallel()

	const sqlStmt = "UPDATE campaigns SET name = $1, tma_url = $2, updated_at = now() WHERE id = $3 RETURNING created_at, id, is_deleted, name, tma_url, updated_at"

	t.Run("success maps row to struct", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}
		createdAt := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
		updatedAt := createdAt.Add(time.Hour)

		mock.ExpectQuery(sqlStmt).
			WithArgs("Promo Y", "https://tma.ugcboost.kz/tz/new", "c-1").
			WillReturnRows(pgxmock.NewRows([]string{"created_at", "id", "is_deleted", "name", "tma_url", "updated_at"}).
				AddRow(createdAt, "c-1", false, "Promo Y", "https://tma.ugcboost.kz/tz/new", updatedAt))

		got, err := repo.Update(context.Background(), "c-1", "Promo Y", "https://tma.ugcboost.kz/tz/new")
		require.NoError(t, err)
		require.Equal(t, &CampaignRow{
			ID:        "c-1",
			Name:      "Promo Y",
			TmaURL:    "https://tma.ugcboost.kz/tz/new",
			IsDeleted: false,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		}, got)
	})

	t.Run("data layer does not filter is_deleted", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}
		createdAt := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
		updatedAt := createdAt.Add(time.Hour)

		// Soft-deleted gate is in the service (UpdateCampaign), not here.
		mock.ExpectQuery(sqlStmt).
			WithArgs("Promo Y", "https://tma.ugcboost.kz/tz/new", "c-2").
			WillReturnRows(pgxmock.NewRows([]string{"created_at", "id", "is_deleted", "name", "tma_url", "updated_at"}).
				AddRow(createdAt, "c-2", true, "Promo Y", "https://tma.ugcboost.kz/tz/new", updatedAt))

		got, err := repo.Update(context.Background(), "c-2", "Promo Y", "https://tma.ugcboost.kz/tz/new")
		require.NoError(t, err)
		require.True(t, got.IsDeleted)
	})

	t.Run("name taken returns ErrCampaignNameTaken", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("Promo Y", "https://tma.ugcboost.kz/tz/new", "c-1").
			WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: CampaignsNameActiveUnique})

		_, err := repo.Update(context.Background(), "c-1", "Promo Y", "https://tma.ugcboost.kz/tz/new")
		require.ErrorIs(t, err, domain.ErrCampaignNameTaken)
	})

	t.Run("unrelated 23505 propagates raw", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}
		pgErr := &pgconn.PgError{Code: "23505", ConstraintName: "campaigns_other_unique"}

		mock.ExpectQuery(sqlStmt).
			WithArgs("Promo Y", "https://tma.ugcboost.kz/tz/new", "c-1").
			WillReturnError(pgErr)

		_, err := repo.Update(context.Background(), "c-1", "Promo Y", "https://tma.ugcboost.kz/tz/new")
		require.NotErrorIs(t, err, domain.ErrCampaignNameTaken)
		require.ErrorIs(t, err, pgErr)
	})

	t.Run("not found propagates sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("Promo Y", "https://tma.ugcboost.kz/tz/new", "missing").
			WillReturnError(sql.ErrNoRows)

		_, err := repo.Update(context.Background(), "missing", "Promo Y", "https://tma.ugcboost.kz/tz/new")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("propagates other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("Promo Y", "https://tma.ugcboost.kz/tz/new", "c-1").
			WillReturnError(errors.New("db unavailable"))

		_, err := repo.Update(context.Background(), "c-1", "Promo Y", "https://tma.ugcboost.kz/tz/new")
		require.ErrorContains(t, err, "db unavailable")
	})
}

func TestCampaignRepository_List(t *testing.T) {
	t.Parallel()

	const countSQLNoFilters = "SELECT COUNT(*) FROM campaigns"
	const pageSelectCols = "SELECT created_at, id, is_deleted, name, tma_url, updated_at"
	const pageFrom = " FROM campaigns"

	t.Run("empty result returns nil 0 nil without page query", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		params := CampaignListParams{
			Sort:    domain.CampaignSortCreatedAt,
			Order:   domain.SortOrderAsc,
			Page:    1,
			PerPage: 10,
		}
		rows, total, err := repo.List(context.Background(), params)
		require.NoError(t, err)
		require.Nil(t, rows)
		require.Zero(t, total)
	})

	t.Run("invalid Page returns error before any SQL is dispatched", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}
		_, _, err := repo.List(context.Background(), CampaignListParams{
			Sort:    domain.CampaignSortCreatedAt,
			Order:   domain.SortOrderAsc,
			Page:    0,
			PerPage: 10,
		})
		require.ErrorContains(t, err, "invalid pagination")
	})

	t.Run("invalid PerPage returns error before any SQL is dispatched", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}
		_, _, err := repo.List(context.Background(), CampaignListParams{
			Sort:    domain.CampaignSortCreatedAt,
			Order:   domain.SortOrderAsc,
			Page:    1,
			PerPage: 0,
		})
		require.ErrorContains(t, err, "invalid pagination")
	})

	t.Run("count query error propagates", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters).
			WillReturnError(errors.New("count failed"))

		_, _, err := repo.List(context.Background(), CampaignListParams{
			Sort:    domain.CampaignSortCreatedAt,
			Order:   domain.SortOrderAsc,
			Page:    1,
			PerPage: 10,
		})
		require.ErrorContains(t, err, "count failed")
	})

	t.Run("page query error propagates", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(5)))
		mock.ExpectQuery(pageSelectCols + pageFrom + " ORDER BY created_at ASC, id ASC LIMIT 10 OFFSET 0").
			WillReturnError(errors.New("page failed"))

		_, _, err := repo.List(context.Background(), CampaignListParams{
			Sort:    domain.CampaignSortCreatedAt,
			Order:   domain.SortOrderAsc,
			Page:    1,
			PerPage: 10,
		})
		require.ErrorContains(t, err, "page failed")
	})

	t.Run("unsupported sort errors after count query, no page query", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}

		// Count returns >0 so the page branch is reachable; the order helper
		// then refuses the unknown sort and the page query never fires.
		mock.ExpectQuery(countSQLNoFilters).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(3)))

		_, _, err := repo.List(context.Background(), CampaignListParams{
			Sort:    "bogus",
			Order:   domain.SortOrderAsc,
			Page:    1,
			PerPage: 10,
		})
		require.ErrorContains(t, err, "unsupported sort")
	})

	t.Run("happy: no filters, sort created_at desc, page 2", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}
		created := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
		updated := created.Add(time.Hour)

		mock.ExpectQuery(countSQLNoFilters).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(25)))
		mock.ExpectQuery(pageSelectCols + pageFrom + " ORDER BY created_at DESC, id ASC LIMIT 10 OFFSET 10").
			WillReturnRows(pgxmock.NewRows([]string{"created_at", "id", "is_deleted", "name", "tma_url", "updated_at"}).
				AddRow(created, "c-1", false, "Promo X", "https://tma.ugcboost.kz/tz/x", updated))

		rows, total, err := repo.List(context.Background(), CampaignListParams{
			Sort:    domain.CampaignSortCreatedAt,
			Order:   domain.SortOrderDesc,
			Page:    2,
			PerPage: 10,
		})
		require.NoError(t, err)
		require.Equal(t, int64(25), total)
		require.Equal(t, []*CampaignRow{{
			ID:        "c-1",
			Name:      "Promo X",
			TmaURL:    "https://tma.ugcboost.kz/tz/x",
			IsDeleted: false,
			CreatedAt: created,
			UpdatedAt: updated,
		}}, rows)
	})

	t.Run("filter: isDeleted=true emits is_deleted = $1 in count", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters + " WHERE is_deleted = $1").
			WithArgs(true).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		isDeleted := true
		_, _, err := repo.List(context.Background(), CampaignListParams{
			IsDeleted: &isDeleted,
			Sort:      domain.CampaignSortCreatedAt,
			Order:     domain.SortOrderAsc,
			Page:      1,
			PerPage:   10,
		})
		require.NoError(t, err)
	})

	t.Run("filter: isDeleted=false emits is_deleted = $1 in count", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters + " WHERE is_deleted = $1").
			WithArgs(false).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		isDeleted := false
		_, _, err := repo.List(context.Background(), CampaignListParams{
			IsDeleted: &isDeleted,
			Sort:      domain.CampaignSortCreatedAt,
			Order:     domain.SortOrderAsc,
			Page:      1,
			PerPage:   10,
		})
		require.NoError(t, err)
	})

	t.Run("filter: search escapes wildcards and uses ILIKE ESCAPE", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}

		// Admin types `100%` — `%` must be neutralised so Postgres treats it
		// as a literal substring rather than the LIKE-any-string wildcard.
		mock.ExpectQuery(countSQLNoFilters + ` WHERE name ILIKE $1 ESCAPE '\'`).
			WithArgs(`%100\%%`).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		_, _, err := repo.List(context.Background(), CampaignListParams{
			Search:  "100%",
			Sort:    domain.CampaignSortCreatedAt,
			Order:   domain.SortOrderAsc,
			Page:    1,
			PerPage: 10,
		})
		require.NoError(t, err)
	})

	t.Run("filter: combined isDeleted + search uses both predicates", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters+` WHERE is_deleted = $1 AND name ILIKE $2 ESCAPE '\'`).
			WithArgs(false, "%promo%").
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		isDeleted := false
		_, _, err := repo.List(context.Background(), CampaignListParams{
			Search:    "promo",
			IsDeleted: &isDeleted,
			Sort:      domain.CampaignSortCreatedAt,
			Order:     domain.SortOrderAsc,
			Page:      1,
			PerPage:   10,
		})
		require.NoError(t, err)
	})

	t.Run("sort: updated_at asc emits updated_at ASC, id ASC tie-breaker", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(1)))
		mock.ExpectQuery(pageSelectCols + pageFrom + " ORDER BY updated_at ASC, id ASC LIMIT 10 OFFSET 0").
			WillReturnRows(pgxmock.NewRows([]string{"created_at", "id", "is_deleted", "name", "tma_url", "updated_at"}))

		_, _, err := repo.List(context.Background(), CampaignListParams{
			Sort:    domain.CampaignSortUpdatedAt,
			Order:   domain.SortOrderAsc,
			Page:    1,
			PerPage: 10,
		})
		require.NoError(t, err)
	})

	t.Run("sort: name desc emits name DESC, id ASC tie-breaker", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(1)))
		mock.ExpectQuery(pageSelectCols + pageFrom + " ORDER BY name DESC, id ASC LIMIT 10 OFFSET 0").
			WillReturnRows(pgxmock.NewRows([]string{"created_at", "id", "is_deleted", "name", "tma_url", "updated_at"}))

		_, _, err := repo.List(context.Background(), CampaignListParams{
			Sort:    domain.CampaignSortName,
			Order:   domain.SortOrderDesc,
			Page:    1,
			PerPage: 10,
		})
		require.NoError(t, err)
	})
}

func TestCampaignRepository_DeleteForTests(t *testing.T) {
	t.Parallel()

	const sqlStmt = "DELETE FROM campaigns WHERE id = $1"

	t.Run("success", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("c-1").
			WillReturnResult(pgconn.NewCommandTag("DELETE 1"))

		err := repo.DeleteForTests(context.Background(), "c-1")
		require.NoError(t, err)
	})

	t.Run("no rows affected returns sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("missing").
			WillReturnResult(pgconn.NewCommandTag("DELETE 0"))

		err := repo.DeleteForTests(context.Background(), "missing")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("propagates other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &campaignRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("c-1").
			WillReturnError(errors.New("db error"))

		err := repo.DeleteForTests(context.Background(), "c-1")
		require.ErrorContains(t, err, "db error")
	})
}

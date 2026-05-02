package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/domain"
)

func TestCreatorApplicationRepository_HasActiveByIIN(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT 1 FROM creator_applications WHERE iin = $1 AND status IN ($2,$3,$4,$5) LIMIT 1"

	t.Run("found returns true", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("950515312348", "verification", "moderation", "awaiting_contract", "contract_sent").
			WillReturnRows(pgxmock.NewRows([]string{"?column?"}).AddRow(1))

		ok, err := repo.HasActiveByIIN(context.Background(), "950515312348")
		require.NoError(t, err)
		require.True(t, ok)
	})

	t.Run("not found returns false without error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("950515312348", "verification", "moderation", "awaiting_contract", "contract_sent").
			WillReturnError(pgx.ErrNoRows)

		ok, err := repo.HasActiveByIIN(context.Background(), "950515312348")
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("propagates other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("950515312348", "verification", "moderation", "awaiting_contract", "contract_sent").
			WillReturnError(errors.New("db exploded"))

		ok, err := repo.HasActiveByIIN(context.Background(), "950515312348")
		require.ErrorContains(t, err, "db exploded")
		require.False(t, ok)
		// sql.ErrNoRows should not be surfaced for this case.
		require.NotErrorIs(t, err, sql.ErrNoRows)
	})
}

func TestCreatorApplicationRepository_Create(t *testing.T) {
	t.Parallel()

	const sqlStmt = "INSERT INTO creator_applications (address,birth_date,category_other_text,city_code,first_name,iin,last_name,middle_name,phone,status) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING address, birth_date, category_other_text, city_code, created_at, first_name, id, iin, last_name, middle_name, phone, status, updated_at"

	t.Run("success returns persisted row", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}
		birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)
		created := time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC)

		// pgx/stom dereferences *string before binding to the SQL parameter, so
		// WithArgs receives the raw string. AddRow goes through the dbutil
		// scanner which requires the source kind to match the destination kind
		// (*string), so the address/middle/other columns are sourced as pointers.
		mock.ExpectQuery(sqlStmt).
			WithArgs("ул. Абая 1", birth, "Авторские ASMR-видео", "almaty", "Айдана", "950515312348", "Муратова", "Ивановна", "+77001234567", "verification").
			WillReturnRows(pgxmock.NewRows([]string{"address", "birth_date", "category_other_text", "city_code", "created_at", "first_name", "id", "iin", "last_name", "middle_name", "phone", "status", "updated_at"}).
				AddRow(pointer.ToString("ул. Абая 1"), birth, pointer.ToString("Авторские ASMR-видео"), "almaty", created, "Айдана", "app-1", "950515312348", "Муратова", pointer.ToString("Ивановна"), "+77001234567", "verification", created))

		row := CreatorApplicationRow{
			LastName:          "Муратова",
			FirstName:         "Айдана",
			MiddleName:        pointer.ToString("Ивановна"),
			IIN:               "950515312348",
			BirthDate:         birth,
			Phone:             "+77001234567",
			CityCode:          "almaty",
			Address:           pointer.ToString("ул. Абая 1"),
			CategoryOtherText: pointer.ToString("Авторские ASMR-видео"),
			Status:            "verification",
		}
		got, err := repo.Create(context.Background(), row)
		require.NoError(t, err)
		require.Equal(t, &CreatorApplicationRow{
			ID:                "app-1",
			LastName:          "Муратова",
			FirstName:         "Айдана",
			MiddleName:        pointer.ToString("Ивановна"),
			IIN:               "950515312348",
			BirthDate:         birth,
			Phone:             "+77001234567",
			CityCode:          "almaty",
			Address:           pointer.ToString("ул. Абая 1"),
			CategoryOtherText: pointer.ToString("Авторские ASMR-видео"),
			Status:            "verification",
			CreatedAt:         created,
			UpdatedAt:         created,
		}, got)
	})

	t.Run("translates pgx unique violation on iin index to domain sentinel", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}
		birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("ул. Абая 1", birth, nil, "almaty", "Айдана", "950515312348", "Муратова", nil, "+77001234567", "verification").
			WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: CreatorApplicationsIINActiveIdx})

		_, err := repo.Create(context.Background(), CreatorApplicationRow{
			LastName:  "Муратова",
			FirstName: "Айдана",
			IIN:       "950515312348",
			BirthDate: birth,
			Phone:     "+77001234567",
			CityCode:  "almaty",
			Address:   pointer.ToString("ул. Абая 1"),
			Status:    "verification",
		})
		require.ErrorIs(t, err, domain.ErrCreatorApplicationDuplicate)
	})

	t.Run("propagates unrelated unique violations as-is", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}
		birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("ул. Абая 1", birth, nil, "almaty", "Айдана", "950515312348", "Муратова", nil, "+77001234567", "verification").
			WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: "some_other_idx"})

		_, err := repo.Create(context.Background(), CreatorApplicationRow{
			LastName:  "Муратова",
			FirstName: "Айдана",
			IIN:       "950515312348",
			BirthDate: birth,
			Phone:     "+77001234567",
			CityCode:  "almaty",
			Address:   pointer.ToString("ул. Абая 1"),
			Status:    "verification",
		})
		require.Error(t, err)
		require.NotErrorIs(t, err, domain.ErrCreatorApplicationDuplicate)
	})

	t.Run("address omitted — repo passes nil to insert and reads it back", func(t *testing.T) {
		t.Parallel()
		// Landing form does not collect an address; the row hits the DB with
		// NULL and pgxmock surfaces it as a nil pointer when scanning back.
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}
		birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)
		created := time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs(nil, birth, nil, "almaty", "Айдана", "950515312348", "Муратова", "Ивановна", "+77001234567", "verification").
			WillReturnRows(pgxmock.NewRows([]string{"address", "birth_date", "category_other_text", "city_code", "created_at", "first_name", "id", "iin", "last_name", "middle_name", "phone", "status", "updated_at"}).
				AddRow(nil, birth, nil, "almaty", created, "Айдана", "app-2", "950515312348", "Муратова", pointer.ToString("Ивановна"), "+77001234567", "verification", created))

		got, err := repo.Create(context.Background(), CreatorApplicationRow{
			LastName:   "Муратова",
			FirstName:  "Айдана",
			MiddleName: pointer.ToString("Ивановна"),
			IIN:        "950515312348",
			BirthDate:  birth,
			Phone:      "+77001234567",
			CityCode:   "almaty",
			Status:     "verification",
		})
		require.NoError(t, err)
		require.Nil(t, got.Address)
	})
}

func TestCreatorApplicationRepository_GetByID(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT address, birth_date, category_other_text, city_code, created_at, first_name, id, iin, last_name, middle_name, phone, status, updated_at FROM creator_applications WHERE id = $1"

	t.Run("success maps row to struct", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}
		birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)
		created := time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC)
		updated := time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC)

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-1").
			WillReturnRows(pgxmock.NewRows([]string{"address", "birth_date", "category_other_text", "city_code", "created_at", "first_name", "id", "iin", "last_name", "middle_name", "phone", "status", "updated_at"}).
				AddRow(pointer.ToString("ул. Абая 1"), birth, pointer.ToString("Авторские ASMR"), "almaty", created, "Айдана", "app-1", "950515312348", "Муратова", pointer.ToString("Ивановна"), "+77001234567", "verification", updated))

		got, err := repo.GetByID(context.Background(), "app-1")
		require.NoError(t, err)
		require.Equal(t, &CreatorApplicationRow{
			ID:                "app-1",
			LastName:          "Муратова",
			FirstName:         "Айдана",
			MiddleName:        pointer.ToString("Ивановна"),
			IIN:               "950515312348",
			BirthDate:         birth,
			Phone:             "+77001234567",
			CityCode:          "almaty",
			Address:           pointer.ToString("ул. Абая 1"),
			CategoryOtherText: pointer.ToString("Авторские ASMR"),
			Status:            "verification",
			CreatedAt:         created,
			UpdatedAt:         updated,
		}, got)
	})

	t.Run("propagates sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("missing").
			WillReturnError(pgx.ErrNoRows)

		_, err := repo.GetByID(context.Background(), "missing")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("propagates other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("app-1").
			WillReturnError(errors.New("db down"))

		_, err := repo.GetByID(context.Background(), "app-1")
		require.ErrorContains(t, err, "db down")
	})
}

func TestCreatorApplicationRepository_List(t *testing.T) {
	t.Parallel()

	// Base SQL fragments — every t.Run composes the count and page queries
	// from these so a literal mismatch surfaces in one place. The cities
	// join is conditional on sort=city_name, so two `pageFrom` variants live
	// here.
	const countSQLNoFilters = "SELECT COUNT(*) FROM creator_applications ca LEFT JOIN creator_application_telegram_links tgl ON tgl.application_id = ca.id"
	const pageSelectCols = "SELECT ca.id AS id, ca.last_name AS last_name, ca.first_name AS first_name, ca.middle_name AS middle_name, ca.birth_date AS birth_date, ca.city_code AS city_code, ca.status AS status, ca.created_at AS created_at, ca.updated_at AS updated_at, (tgl.application_id IS NOT NULL) AS telegram_linked"
	const pageFrom = " FROM creator_applications ca LEFT JOIN creator_application_telegram_links tgl ON tgl.application_id = ca.id"
	const pageFromWithCity = pageFrom + " LEFT JOIN cities ct ON ct.code = ca.city_code"

	t.Run("empty result returns nil 0 nil without page query", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		params := CreatorApplicationListParams{
			Sort:    domain.CreatorApplicationSortCreatedAt,
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
		repo := &creatorApplicationRepository{db: mock}
		_, _, err := repo.List(context.Background(), CreatorApplicationListParams{
			Sort:    domain.CreatorApplicationSortCreatedAt,
			Order:   domain.SortOrderAsc,
			Page:    0,
			PerPage: 10,
		})
		require.ErrorContains(t, err, "invalid pagination")
	})

	t.Run("invalid PerPage returns error before any SQL is dispatched", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}
		_, _, err := repo.List(context.Background(), CreatorApplicationListParams{
			Sort:    domain.CreatorApplicationSortCreatedAt,
			Order:   domain.SortOrderAsc,
			Page:    1,
			PerPage: 0,
		})
		require.ErrorContains(t, err, "invalid pagination")
	})

	t.Run("count query error propagates", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters).
			WillReturnError(errors.New("count failed"))

		_, _, err := repo.List(context.Background(), CreatorApplicationListParams{
			Sort:    domain.CreatorApplicationSortCreatedAt,
			Order:   domain.SortOrderAsc,
			Page:    1,
			PerPage: 10,
		})
		require.ErrorContains(t, err, "count failed")
	})

	t.Run("page query error propagates", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(5)))
		mock.ExpectQuery(pageSelectCols + pageFrom + " ORDER BY ca.created_at ASC, ca.id ASC LIMIT 10 OFFSET 0").
			WillReturnError(errors.New("page failed"))

		_, _, err := repo.List(context.Background(), CreatorApplicationListParams{
			Sort:    domain.CreatorApplicationSortCreatedAt,
			Order:   domain.SortOrderAsc,
			Page:    1,
			PerPage: 10,
		})
		require.ErrorContains(t, err, "page failed")
	})

	t.Run("happy: no filters, sort created_at desc, page 2", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}
		birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)
		created := time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC)
		updated := time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC)

		mock.ExpectQuery(countSQLNoFilters).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(25)))
		mock.ExpectQuery(pageSelectCols + pageFrom + " ORDER BY ca.created_at DESC, ca.id ASC LIMIT 10 OFFSET 10").
			WillReturnRows(pgxmock.NewRows([]string{"id", "last_name", "first_name", "middle_name", "birth_date", "city_code", "status", "created_at", "updated_at", "telegram_linked"}).
				AddRow("app-1", "Муратова", "Айдана", pointer.ToString("Ивановна"), birth, "almaty", "verification", created, updated, true))

		rows, total, err := repo.List(context.Background(), CreatorApplicationListParams{
			Sort:    domain.CreatorApplicationSortCreatedAt,
			Order:   domain.SortOrderDesc,
			Page:    2,
			PerPage: 10,
		})
		require.NoError(t, err)
		require.Equal(t, int64(25), total)
		require.Equal(t, []*CreatorApplicationListRow{{
			ID:             "app-1",
			LastName:       "Муратова",
			FirstName:      "Айдана",
			MiddleName:     pointer.ToString("Ивановна"),
			BirthDate:      birth,
			CityCode:       "almaty",
			Status:         "verification",
			CreatedAt:      created,
			UpdatedAt:      updated,
			TelegramLinked: true,
		}}, rows)
	})

	t.Run("filter: statuses array adds IN clause on count and page", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters+" WHERE ca.status IN ($1,$2)").
			WithArgs("verification", "moderation").
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		_, _, err := repo.List(context.Background(), CreatorApplicationListParams{
			Statuses: []string{"verification", "moderation"},
			Sort:     domain.CreatorApplicationSortCreatedAt,
			Order:    domain.SortOrderAsc,
			Page:     1,
			PerPage:  10,
		})
		require.NoError(t, err)
	})

	t.Run("filter: cities array adds IN clause", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters+" WHERE ca.city_code IN ($1,$2)").
			WithArgs("almaty", "astana").
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		_, _, err := repo.List(context.Background(), CreatorApplicationListParams{
			Cities:  []string{"almaty", "astana"},
			Sort:    domain.CreatorApplicationSortCreatedAt,
			Order:   domain.SortOrderAsc,
			Page:    1,
			PerPage: 10,
		})
		require.NoError(t, err)
	})

	t.Run("filter: categories adds EXISTS subquery", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters+" WHERE EXISTS (SELECT 1 FROM creator_application_categories cac WHERE cac.application_id = ca.id AND cac.category_code IN ($1,$2))").
			WithArgs("beauty", "fashion").
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		_, _, err := repo.List(context.Background(), CreatorApplicationListParams{
			Categories: []string{"beauty", "fashion"},
			Sort:       domain.CreatorApplicationSortCreatedAt,
			Order:      domain.SortOrderAsc,
			Page:       1,
			PerPage:    10,
		})
		require.NoError(t, err)
	})

	t.Run("filter: dateFrom and dateTo add created_at range", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}
		from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)

		mock.ExpectQuery(countSQLNoFilters+" WHERE ca.created_at >= $1 AND ca.created_at <= $2").
			WithArgs(from, to).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		_, _, err := repo.List(context.Background(), CreatorApplicationListParams{
			DateFrom: &from,
			DateTo:   &to,
			Sort:     domain.CreatorApplicationSortCreatedAt,
			Order:    domain.SortOrderAsc,
			Page:     1,
			PerPage:  10,
		})
		require.NoError(t, err)
	})

	t.Run("filter: ageFrom adds birth_date math", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters + " WHERE ca.birth_date <= NOW()::date - make_interval(years => $1)").
			WithArgs(18).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		ageFrom := 18
		_, _, err := repo.List(context.Background(), CreatorApplicationListParams{
			AgeFrom: &ageFrom,
			Sort:    domain.CreatorApplicationSortCreatedAt,
			Order:   domain.SortOrderAsc,
			Page:    1,
			PerPage: 10,
		})
		require.NoError(t, err)
	})

	t.Run("filter: ageTo bumps interval by one year", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters + " WHERE ca.birth_date > NOW()::date - make_interval(years => $1)").
			WithArgs(36).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		ageTo := 35
		_, _, err := repo.List(context.Background(), CreatorApplicationListParams{
			AgeTo:   &ageTo,
			Sort:    domain.CreatorApplicationSortCreatedAt,
			Order:   domain.SortOrderAsc,
			Page:    1,
			PerPage: 10,
		})
		require.NoError(t, err)
	})

	t.Run("filter: telegramLinked true adds IS NOT NULL", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters + " WHERE tgl.application_id IS NOT NULL").
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		linked := true
		_, _, err := repo.List(context.Background(), CreatorApplicationListParams{
			TelegramLinked: &linked,
			Sort:           domain.CreatorApplicationSortCreatedAt,
			Order:          domain.SortOrderAsc,
			Page:           1,
			PerPage:        10,
		})
		require.NoError(t, err)
	})

	t.Run("filter: telegramLinked false adds IS NULL", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters + " WHERE tgl.application_id IS NULL").
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		linked := false
		_, _, err := repo.List(context.Background(), CreatorApplicationListParams{
			TelegramLinked: &linked,
			Sort:           domain.CreatorApplicationSortCreatedAt,
			Order:          domain.SortOrderAsc,
			Page:           1,
			PerPage:        10,
		})
		require.NoError(t, err)
	})

	t.Run("filter: search adds ESCAPE'd ILIKE chain plus EXISTS over socials", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		// Five $-placeholders feeding the same %pattern% — four columns plus
		// the EXISTS handle subquery. Each ILIKE pairs with `ESCAPE '\'` so
		// LIKE wildcards in user input are treated as literals.
		mock.ExpectQuery(countSQLNoFilters+` WHERE (ca.last_name ILIKE $1 ESCAPE '\' OR ca.first_name ILIKE $2 ESCAPE '\' OR ca.middle_name ILIKE $3 ESCAPE '\' OR ca.iin ILIKE $4 ESCAPE '\' OR EXISTS (SELECT 1 FROM creator_application_socials cas WHERE cas.application_id = ca.id AND cas.handle ILIKE $5 ESCAPE '\'))`).
			WithArgs("%aidana%", "%aidana%", "%aidana%", "%aidana%", "%aidana%").
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		_, _, err := repo.List(context.Background(), CreatorApplicationListParams{
			Search:  "aidana",
			Sort:    domain.CreatorApplicationSortCreatedAt,
			Order:   domain.SortOrderAsc,
			Page:    1,
			PerPage: 10,
		})
		require.NoError(t, err)
	})

	t.Run("filter: search escapes LIKE wildcards in user input", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		// Input "100%_user\\admin" — backslash first (so we don't double-
		// escape the escapes we just inserted), then percent, then underscore.
		// Final pattern wrapping in %...% on both sides for substring match.
		mock.ExpectQuery(countSQLNoFilters+` WHERE (ca.last_name ILIKE $1 ESCAPE '\' OR ca.first_name ILIKE $2 ESCAPE '\' OR ca.middle_name ILIKE $3 ESCAPE '\' OR ca.iin ILIKE $4 ESCAPE '\' OR EXISTS (SELECT 1 FROM creator_application_socials cas WHERE cas.application_id = ca.id AND cas.handle ILIKE $5 ESCAPE '\'))`).
			WithArgs(`%100\%\_user\\admin%`, `%100\%\_user\\admin%`, `%100\%\_user\\admin%`, `%100\%\_user\\admin%`, `%100\%\_user\\admin%`).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		_, _, err := repo.List(context.Background(), CreatorApplicationListParams{
			Search:  `100%_user\admin`,
			Sort:    domain.CreatorApplicationSortCreatedAt,
			Order:   domain.SortOrderAsc,
			Page:    1,
			PerPage: 10,
		})
		require.NoError(t, err)
	})

	t.Run("sort: city_name asc orders by ct.name", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(1)))
		mock.ExpectQuery(pageSelectCols + pageFromWithCity + " ORDER BY ct.name ASC, ca.id ASC LIMIT 10 OFFSET 0").
			WillReturnRows(pgxmock.NewRows([]string{"id", "last_name", "first_name", "middle_name", "birth_date", "city_code", "status", "created_at", "updated_at", "telegram_linked"}))

		_, _, err := repo.List(context.Background(), CreatorApplicationListParams{
			Sort:    domain.CreatorApplicationSortCityName,
			Order:   domain.SortOrderAsc,
			Page:    1,
			PerPage: 10,
		})
		require.NoError(t, err)
	})

	t.Run("sort: full_name asc orders by last/first/middle/id", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(1)))
		mock.ExpectQuery(pageSelectCols + pageFrom + " ORDER BY ca.last_name ASC, ca.first_name ASC, ca.middle_name ASC, ca.id ASC LIMIT 10 OFFSET 0").
			WillReturnRows(pgxmock.NewRows([]string{"id", "last_name", "first_name", "middle_name", "birth_date", "city_code", "status", "created_at", "updated_at", "telegram_linked"}))

		_, _, err := repo.List(context.Background(), CreatorApplicationListParams{
			Sort:    domain.CreatorApplicationSortFullName,
			Order:   domain.SortOrderAsc,
			Page:    1,
			PerPage: 10,
		})
		require.NoError(t, err)
	})

	t.Run("sort: birth_date desc orders by ca.birth_date DESC then id", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(1)))
		mock.ExpectQuery(pageSelectCols + pageFrom + " ORDER BY ca.birth_date DESC, ca.id ASC LIMIT 10 OFFSET 0").
			WillReturnRows(pgxmock.NewRows([]string{"id", "last_name", "first_name", "middle_name", "birth_date", "city_code", "status", "created_at", "updated_at", "telegram_linked"}))

		_, _, err := repo.List(context.Background(), CreatorApplicationListParams{
			Sort:    domain.CreatorApplicationSortBirthDate,
			Order:   domain.SortOrderDesc,
			Page:    1,
			PerPage: 10,
		})
		require.NoError(t, err)
	})

	t.Run("sort: updated_at asc", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(1)))
		mock.ExpectQuery(pageSelectCols + pageFrom + " ORDER BY ca.updated_at ASC, ca.id ASC LIMIT 10 OFFSET 0").
			WillReturnRows(pgxmock.NewRows([]string{"id", "last_name", "first_name", "middle_name", "birth_date", "city_code", "status", "created_at", "updated_at", "telegram_linked"}))

		_, _, err := repo.List(context.Background(), CreatorApplicationListParams{
			Sort:    domain.CreatorApplicationSortUpdatedAt,
			Order:   domain.SortOrderAsc,
			Page:    1,
			PerPage: 10,
		})
		require.NoError(t, err)
	})

	t.Run("sort: unknown value falls back to created_at desc (defensive)", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(1)))
		mock.ExpectQuery(pageSelectCols + pageFrom + " ORDER BY ca.created_at DESC, ca.id ASC LIMIT 10 OFFSET 0").
			WillReturnRows(pgxmock.NewRows([]string{"id", "last_name", "first_name", "middle_name", "birth_date", "city_code", "status", "created_at", "updated_at", "telegram_linked"}))

		_, _, err := repo.List(context.Background(), CreatorApplicationListParams{
			Sort:    "rating", // service rejects this; repo defensively falls back
			Order:   domain.SortOrderAsc,
			Page:    1,
			PerPage: 10,
		})
		require.NoError(t, err)
	})
}

func TestCreatorApplicationRepository_DeleteForTests(t *testing.T) {
	t.Parallel()

	const sqlStmt = "DELETE FROM creator_applications WHERE id = $1"

	t.Run("success returns nil", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("app-1").
			WillReturnResult(pgconn.NewCommandTag("DELETE 1"))

		require.NoError(t, repo.DeleteForTests(context.Background(), "app-1"))
	})

	t.Run("missing returns sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("missing").
			WillReturnResult(pgconn.NewCommandTag("DELETE 0"))

		err := repo.DeleteForTests(context.Background(), "missing")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("propagates db error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorApplicationRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("app-1").
			WillReturnError(errors.New("db down"))

		err := repo.DeleteForTests(context.Background(), "app-1")
		require.ErrorContains(t, err, "db down")
	})
}

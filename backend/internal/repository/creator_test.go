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

func TestCreatorRepository_List(t *testing.T) {
	t.Parallel()

	const countSQLNoFilters = "SELECT COUNT(*) FROM creators cr"
	const pageSelectCols = "SELECT cr.birth_date AS birth_date, cr.city_code AS city_code, cr.created_at AS created_at, cr.first_name AS first_name, cr.id AS id, cr.iin AS iin, cr.last_name AS last_name, cr.middle_name AS middle_name, cr.phone AS phone, cr.telegram_username AS telegram_username, cr.updated_at AS updated_at"
	const pageFrom = " FROM creators cr"
	const pageFromWithCity = pageFrom + " LEFT JOIN cities ct ON ct.code = cr.city_code"

	t.Run("empty result returns nil 0 nil without page query", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		params := CreatorListParams{
			Sort:    domain.CreatorSortCreatedAt,
			Order:   domain.SortOrderAsc,
			Page:    1,
			PerPage: 10,
		}
		rows, total, err := repo.List(context.Background(), params)
		require.NoError(t, err)
		require.Nil(t, rows)
		require.Zero(t, total)
	})

	t.Run("invalid Page returns error before any SQL", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}
		_, _, err := repo.List(context.Background(), CreatorListParams{
			Sort: domain.CreatorSortCreatedAt, Order: domain.SortOrderAsc, Page: 0, PerPage: 10,
		})
		require.ErrorContains(t, err, "invalid pagination")
	})

	t.Run("invalid PerPage returns error before any SQL", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}
		_, _, err := repo.List(context.Background(), CreatorListParams{
			Sort: domain.CreatorSortCreatedAt, Order: domain.SortOrderAsc, Page: 1, PerPage: 0,
		})
		require.ErrorContains(t, err, "invalid pagination")
	})

	t.Run("count query error propagates", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters).
			WillReturnError(errors.New("count failed"))

		_, _, err := repo.List(context.Background(), CreatorListParams{
			Sort: domain.CreatorSortCreatedAt, Order: domain.SortOrderAsc, Page: 1, PerPage: 10,
		})
		require.ErrorContains(t, err, "count failed")
	})

	t.Run("page query error propagates", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(5)))
		mock.ExpectQuery(pageSelectCols + pageFrom + " ORDER BY cr.created_at ASC, cr.id ASC LIMIT 10 OFFSET 0").
			WillReturnError(errors.New("page failed"))

		_, _, err := repo.List(context.Background(), CreatorListParams{
			Sort: domain.CreatorSortCreatedAt, Order: domain.SortOrderAsc, Page: 1, PerPage: 10,
		})
		require.ErrorContains(t, err, "page failed")
	})

	t.Run("happy: no filters, sort created_at desc, page 2", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}
		birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)
		created := time.Date(2026, 4, 20, 18, 0, 0, 0, time.UTC)
		updated := time.Date(2026, 4, 21, 9, 0, 0, 0, time.UTC)

		mock.ExpectQuery(countSQLNoFilters).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(25)))
		mock.ExpectQuery(pageSelectCols + pageFrom + " ORDER BY cr.created_at DESC, cr.id ASC LIMIT 10 OFFSET 10").
			WillReturnRows(pgxmock.NewRows([]string{"id", "last_name", "first_name", "middle_name", "iin", "birth_date", "phone", "city_code", "telegram_username", "created_at", "updated_at"}).
				AddRow("creator-1", "Муратова", "Айдана", pointer.ToString("Ивановна"), "950515312348", birth, "+77001234567", "almaty", pointer.ToString("aidana_tg"), created, updated))

		rows, total, err := repo.List(context.Background(), CreatorListParams{
			Sort: domain.CreatorSortCreatedAt, Order: domain.SortOrderDesc, Page: 2, PerPage: 10,
		})
		require.NoError(t, err)
		require.Equal(t, int64(25), total)
		require.Equal(t, []*CreatorListRow{{
			ID:               "creator-1",
			LastName:         "Муратова",
			FirstName:        "Айдана",
			MiddleName:       pointer.ToString("Ивановна"),
			IIN:              "950515312348",
			BirthDate:        birth,
			Phone:            "+77001234567",
			CityCode:         "almaty",
			TelegramUsername: pointer.ToString("aidana_tg"),
			CreatedAt:        created,
			UpdatedAt:        updated,
		}}, rows)
	})

	t.Run("filter: cities array adds IN clause", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters+" WHERE cr.city_code IN ($1,$2)").
			WithArgs("almaty", "astana").
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		_, _, err := repo.List(context.Background(), CreatorListParams{
			Cities: []string{"almaty", "astana"},
			Sort:   domain.CreatorSortCreatedAt, Order: domain.SortOrderAsc, Page: 1, PerPage: 10,
		})
		require.NoError(t, err)
	})

	t.Run("filter: categories adds EXISTS subquery", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters+" WHERE EXISTS (SELECT 1 FROM creator_categories ccat WHERE ccat.creator_id = cr.id AND ccat.category_code IN ($1,$2))").
			WithArgs("beauty", "fashion").
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		_, _, err := repo.List(context.Background(), CreatorListParams{
			Categories: []string{"beauty", "fashion"},
			Sort:       domain.CreatorSortCreatedAt, Order: domain.SortOrderAsc, Page: 1, PerPage: 10,
		})
		require.NoError(t, err)
	})

	t.Run("filter: dateFrom and dateTo add created_at range", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}
		from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)

		mock.ExpectQuery(countSQLNoFilters+" WHERE cr.created_at >= $1 AND cr.created_at <= $2").
			WithArgs(from, to).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		_, _, err := repo.List(context.Background(), CreatorListParams{
			DateFrom: &from, DateTo: &to,
			Sort: domain.CreatorSortCreatedAt, Order: domain.SortOrderAsc, Page: 1, PerPage: 10,
		})
		require.NoError(t, err)
	})

	t.Run("filter: ageFrom adds birth_date math", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters + " WHERE cr.birth_date <= NOW()::date - make_interval(years => $1)").
			WithArgs(18).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		ageFrom := 18
		_, _, err := repo.List(context.Background(), CreatorListParams{
			AgeFrom: &ageFrom,
			Sort:    domain.CreatorSortCreatedAt, Order: domain.SortOrderAsc, Page: 1, PerPage: 10,
		})
		require.NoError(t, err)
	})

	t.Run("filter: ageTo bumps interval by one year", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters + " WHERE cr.birth_date > NOW()::date - make_interval(years => $1)").
			WithArgs(36).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		ageTo := 35
		_, _, err := repo.List(context.Background(), CreatorListParams{
			AgeTo: &ageTo,
			Sort:  domain.CreatorSortCreatedAt, Order: domain.SortOrderAsc, Page: 1, PerPage: 10,
		})
		require.NoError(t, err)
	})

	t.Run("filter: search adds ESCAPE'd ILIKE chain plus EXISTS over socials", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		// Seven $-placeholders feeding the same %pattern% — six creator
		// columns (last/first/middle name, IIN, phone, telegram_username) plus
		// the EXISTS handle subquery. Each ILIKE pairs with ESCAPE so LIKE
		// wildcards in user input are treated as literals.
		mock.ExpectQuery(countSQLNoFilters+` WHERE (cr.last_name ILIKE $1 ESCAPE '\' OR cr.first_name ILIKE $2 ESCAPE '\' OR cr.middle_name ILIKE $3 ESCAPE '\' OR cr.iin ILIKE $4 ESCAPE '\' OR cr.phone ILIKE $5 ESCAPE '\' OR cr.telegram_username ILIKE $6 ESCAPE '\' OR EXISTS (SELECT 1 FROM creator_socials csoc WHERE csoc.creator_id = cr.id AND csoc.handle ILIKE $7 ESCAPE '\'))`).
			WithArgs("%aidana%", "%aidana%", "%aidana%", "%aidana%", "%aidana%", "%aidana%", "%aidana%").
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		_, _, err := repo.List(context.Background(), CreatorListParams{
			Search: "aidana",
			Sort:   domain.CreatorSortCreatedAt, Order: domain.SortOrderAsc, Page: 1, PerPage: 10,
		})
		require.NoError(t, err)
	})

	t.Run("filter: search escapes LIKE wildcards in user input", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters+` WHERE (cr.last_name ILIKE $1 ESCAPE '\' OR cr.first_name ILIKE $2 ESCAPE '\' OR cr.middle_name ILIKE $3 ESCAPE '\' OR cr.iin ILIKE $4 ESCAPE '\' OR cr.phone ILIKE $5 ESCAPE '\' OR cr.telegram_username ILIKE $6 ESCAPE '\' OR EXISTS (SELECT 1 FROM creator_socials csoc WHERE csoc.creator_id = cr.id AND csoc.handle ILIKE $7 ESCAPE '\'))`).
			WithArgs(`%100\%\_user\\admin%`, `%100\%\_user\\admin%`, `%100\%\_user\\admin%`, `%100\%\_user\\admin%`, `%100\%\_user\\admin%`, `%100\%\_user\\admin%`, `%100\%\_user\\admin%`).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(0)))

		_, _, err := repo.List(context.Background(), CreatorListParams{
			Search: `100%_user\admin`,
			Sort:   domain.CreatorSortCreatedAt, Order: domain.SortOrderAsc, Page: 1, PerPage: 10,
		})
		require.NoError(t, err)
	})

	t.Run("sort: city_name asc orders by ct.name", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(1)))
		mock.ExpectQuery(pageSelectCols + pageFromWithCity + " ORDER BY ct.name ASC, cr.id ASC LIMIT 10 OFFSET 0").
			WillReturnRows(pgxmock.NewRows([]string{"id", "last_name", "first_name", "middle_name", "iin", "birth_date", "phone", "city_code", "telegram_username", "created_at", "updated_at"}))

		_, _, err := repo.List(context.Background(), CreatorListParams{
			Sort: domain.CreatorSortCityName, Order: domain.SortOrderAsc, Page: 1, PerPage: 10,
		})
		require.NoError(t, err)
	})

	t.Run("sort: full_name asc orders by last/first/middle/id", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(1)))
		mock.ExpectQuery(pageSelectCols + pageFrom + " ORDER BY cr.last_name ASC, cr.first_name ASC, cr.middle_name ASC, cr.id ASC LIMIT 10 OFFSET 0").
			WillReturnRows(pgxmock.NewRows([]string{"id", "last_name", "first_name", "middle_name", "iin", "birth_date", "phone", "city_code", "telegram_username", "created_at", "updated_at"}))

		_, _, err := repo.List(context.Background(), CreatorListParams{
			Sort: domain.CreatorSortFullName, Order: domain.SortOrderAsc, Page: 1, PerPage: 10,
		})
		require.NoError(t, err)
	})

	t.Run("sort: birth_date desc orders by cr.birth_date DESC then id", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(1)))
		mock.ExpectQuery(pageSelectCols + pageFrom + " ORDER BY cr.birth_date DESC, cr.id ASC LIMIT 10 OFFSET 0").
			WillReturnRows(pgxmock.NewRows([]string{"id", "last_name", "first_name", "middle_name", "iin", "birth_date", "phone", "city_code", "telegram_username", "created_at", "updated_at"}))

		_, _, err := repo.List(context.Background(), CreatorListParams{
			Sort: domain.CreatorSortBirthDate, Order: domain.SortOrderDesc, Page: 1, PerPage: 10,
		})
		require.NoError(t, err)
	})

	t.Run("sort: updated_at asc", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(1)))
		mock.ExpectQuery(pageSelectCols + pageFrom + " ORDER BY cr.updated_at ASC, cr.id ASC LIMIT 10 OFFSET 0").
			WillReturnRows(pgxmock.NewRows([]string{"id", "last_name", "first_name", "middle_name", "iin", "birth_date", "phone", "city_code", "telegram_username", "created_at", "updated_at"}))

		_, _, err := repo.List(context.Background(), CreatorListParams{
			Sort: domain.CreatorSortUpdatedAt, Order: domain.SortOrderAsc, Page: 1, PerPage: 10,
		})
		require.NoError(t, err)
	})

	t.Run("sort: unknown value returns error after count succeeds", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(countSQLNoFilters).
			WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(int64(1)))

		_, _, err := repo.List(context.Background(), CreatorListParams{
			Sort: "rating", Order: domain.SortOrderAsc, Page: 1, PerPage: 10,
		})
		require.ErrorContains(t, err, `unsupported sort "rating"`)
	})
}

func TestCreatorRepository_Create(t *testing.T) {
	t.Parallel()

	const sqlStmt = "INSERT INTO creators (address,birth_date,category_other_text,city_code,first_name,iin,last_name,middle_name,phone,source_application_id,telegram_first_name,telegram_last_name,telegram_user_id,telegram_username) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14) RETURNING address, birth_date, category_other_text, city_code, created_at, first_name, id, iin, last_name, middle_name, phone, source_application_id, telegram_first_name, telegram_last_name, telegram_user_id, telegram_username, updated_at"

	birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)
	created := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)

	rowFull := CreatorRow{
		IIN:                 "950515312348",
		LastName:            "Муратова",
		FirstName:           "Айдана",
		MiddleName:          pointer.ToString("Ивановна"),
		BirthDate:           birth,
		Phone:               "+77001234567",
		CityCode:            "almaty",
		Address:             pointer.ToString("ул. Абая 1"),
		CategoryOtherText:   pointer.ToString("ASMR"),
		TelegramUserID:      9000000001,
		TelegramUsername:    pointer.ToString("aidana"),
		TelegramFirstName:   pointer.ToString("Aidana"),
		TelegramLastName:    pointer.ToString("M."),
		SourceApplicationID: "app-1",
	}

	t.Run("success returns persisted row", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("ул. Абая 1", birth, "ASMR", "almaty", "Айдана", "950515312348", "Муратова", "Ивановна", "+77001234567", "app-1", "Aidana", "M.", int64(9000000001), "aidana").
			WillReturnRows(pgxmock.NewRows([]string{"address", "birth_date", "category_other_text", "city_code", "created_at", "first_name", "id", "iin", "last_name", "middle_name", "phone", "source_application_id", "telegram_first_name", "telegram_last_name", "telegram_user_id", "telegram_username", "updated_at"}).
				AddRow(pointer.ToString("ул. Абая 1"), birth, pointer.ToString("ASMR"), "almaty", created, "Айдана", "creator-1", "950515312348", "Муратова", pointer.ToString("Ивановна"), "+77001234567", "app-1", pointer.ToString("Aidana"), pointer.ToString("M."), int64(9000000001), pointer.ToString("aidana"), created))

		got, err := repo.Create(context.Background(), rowFull)
		require.NoError(t, err)
		require.Equal(t, &CreatorRow{
			ID:                  "creator-1",
			IIN:                 "950515312348",
			LastName:            "Муратова",
			FirstName:           "Айдана",
			MiddleName:          pointer.ToString("Ивановна"),
			BirthDate:           birth,
			Phone:               "+77001234567",
			CityCode:            "almaty",
			Address:             pointer.ToString("ул. Абая 1"),
			CategoryOtherText:   pointer.ToString("ASMR"),
			TelegramUserID:      9000000001,
			TelegramUsername:    pointer.ToString("aidana"),
			TelegramFirstName:   pointer.ToString("Aidana"),
			TelegramLastName:    pointer.ToString("M."),
			SourceApplicationID: "app-1",
			CreatedAt:           created,
			UpdatedAt:           created,
		}, got)
	})

	t.Run("nullable columns are passed as nil and read back as nil", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}
		row := CreatorRow{
			IIN:                 "950515312349",
			LastName:            "Муратова",
			FirstName:           "Айдана",
			BirthDate:           birth,
			Phone:               "+77001234567",
			CityCode:            "almaty",
			TelegramUserID:      9000000002,
			SourceApplicationID: "app-2",
		}

		mock.ExpectQuery(sqlStmt).
			WithArgs(nil, birth, nil, "almaty", "Айдана", "950515312349", "Муратова", nil, "+77001234567", "app-2", nil, nil, int64(9000000002), nil).
			WillReturnRows(pgxmock.NewRows([]string{"address", "birth_date", "category_other_text", "city_code", "created_at", "first_name", "id", "iin", "last_name", "middle_name", "phone", "source_application_id", "telegram_first_name", "telegram_last_name", "telegram_user_id", "telegram_username", "updated_at"}).
				AddRow(nil, birth, nil, "almaty", created, "Айдана", "creator-2", "950515312349", "Муратова", nil, "+77001234567", "app-2", nil, nil, int64(9000000002), nil, created))

		got, err := repo.Create(context.Background(), row)
		require.NoError(t, err)
		require.Nil(t, got.MiddleName)
		require.Nil(t, got.Address)
		require.Nil(t, got.CategoryOtherText)
		require.Nil(t, got.TelegramUsername)
		require.Nil(t, got.TelegramFirstName)
		require.Nil(t, got.TelegramLastName)
	})

	t.Run("translates 23505 on creators_iin_unique to ErrCreatorAlreadyExists", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("ул. Абая 1", birth, "ASMR", "almaty", "Айдана", "950515312348", "Муратова", "Ивановна", "+77001234567", "app-1", "Aidana", "M.", int64(9000000001), "aidana").
			WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: CreatorsIINUnique})

		_, err := repo.Create(context.Background(), rowFull)
		require.ErrorIs(t, err, domain.ErrCreatorAlreadyExists)
	})

	t.Run("translates 23505 on creators_telegram_user_id_unique to ErrCreatorTelegramAlreadyTaken", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("ул. Абая 1", birth, "ASMR", "almaty", "Айдана", "950515312348", "Муратова", "Ивановна", "+77001234567", "app-1", "Aidana", "M.", int64(9000000001), "aidana").
			WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: CreatorsTelegramUserIDUnique})

		_, err := repo.Create(context.Background(), rowFull)
		require.ErrorIs(t, err, domain.ErrCreatorTelegramAlreadyTaken)
	})

	t.Run("translates 23505 on creators_source_application_id_unique to ErrCreatorApplicationNotApprovable", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("ул. Абая 1", birth, "ASMR", "almaty", "Айдана", "950515312348", "Муратова", "Ивановна", "+77001234567", "app-1", "Aidana", "M.", int64(9000000001), "aidana").
			WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: CreatorsSourceApplicationIDUnique})

		_, err := repo.Create(context.Background(), rowFull)
		require.ErrorIs(t, err, domain.ErrCreatorApplicationNotApprovable)
	})

	t.Run("propagates unrelated 23505 violations as-is", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("ул. Абая 1", birth, "ASMR", "almaty", "Айдана", "950515312348", "Муратова", "Ивановна", "+77001234567", "app-1", "Aidana", "M.", int64(9000000001), "aidana").
			WillReturnError(&pgconn.PgError{Code: "23505", ConstraintName: "some_other_idx"})

		_, err := repo.Create(context.Background(), rowFull)
		require.Error(t, err)
		require.NotErrorIs(t, err, domain.ErrCreatorAlreadyExists)
		require.NotErrorIs(t, err, domain.ErrCreatorTelegramAlreadyTaken)
		require.NotErrorIs(t, err, domain.ErrCreatorApplicationNotApprovable)
	})

	t.Run("propagates non-unique pg errors with context", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("ул. Абая 1", birth, "ASMR", "almaty", "Айдана", "950515312348", "Муратова", "Ивановна", "+77001234567", "app-1", "Aidana", "M.", int64(9000000001), "aidana").
			WillReturnError(&pgconn.PgError{Code: "23503", ConstraintName: "creators_source_application_id_fkey", Message: "FK violation"})

		_, err := repo.Create(context.Background(), rowFull)
		require.Error(t, err)
		require.NotErrorIs(t, err, domain.ErrCreatorAlreadyExists)
	})

	t.Run("propagates generic db errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("ул. Абая 1", birth, "ASMR", "almaty", "Айдана", "950515312348", "Муратова", "Ивановна", "+77001234567", "app-1", "Aidana", "M.", int64(9000000001), "aidana").
			WillReturnError(errors.New("connection refused"))

		_, err := repo.Create(context.Background(), rowFull)
		require.ErrorContains(t, err, "connection refused")
	})
}

func TestCreatorRepository_GetByID(t *testing.T) {
	t.Parallel()

	const sqlStmt = "SELECT address, birth_date, category_other_text, city_code, created_at, first_name, id, iin, last_name, middle_name, phone, source_application_id, telegram_first_name, telegram_last_name, telegram_user_id, telegram_username, updated_at FROM creators WHERE id = $1"

	birth := time.Date(1995, 5, 15, 0, 0, 0, 0, time.UTC)
	created := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)

	t.Run("success maps row to struct", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("creator-1").
			WillReturnRows(pgxmock.NewRows([]string{"address", "birth_date", "category_other_text", "city_code", "created_at", "first_name", "id", "iin", "last_name", "middle_name", "phone", "source_application_id", "telegram_first_name", "telegram_last_name", "telegram_user_id", "telegram_username", "updated_at"}).
				AddRow(pointer.ToString("ул. Абая 1"), birth, pointer.ToString("ASMR"), "almaty", created, "Айдана", "creator-1", "950515312348", "Муратова", pointer.ToString("Ивановна"), "+77001234567", "app-1", pointer.ToString("Aidana"), pointer.ToString("M."), int64(9000000001), pointer.ToString("aidana"), created))

		got, err := repo.GetByID(context.Background(), "creator-1")
		require.NoError(t, err)
		require.Equal(t, &CreatorRow{
			ID:                  "creator-1",
			IIN:                 "950515312348",
			LastName:            "Муратова",
			FirstName:           "Айдана",
			MiddleName:          pointer.ToString("Ивановна"),
			BirthDate:           birth,
			Phone:               "+77001234567",
			CityCode:            "almaty",
			Address:             pointer.ToString("ул. Абая 1"),
			CategoryOtherText:   pointer.ToString("ASMR"),
			TelegramUserID:      9000000001,
			TelegramUsername:    pointer.ToString("aidana"),
			TelegramFirstName:   pointer.ToString("Aidana"),
			TelegramLastName:    pointer.ToString("M."),
			SourceApplicationID: "app-1",
			CreatedAt:           created,
			UpdatedAt:           created,
		}, got)
	})

	t.Run("propagates sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("missing").
			WillReturnError(pgx.ErrNoRows)

		_, err := repo.GetByID(context.Background(), "missing")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("propagates other errors", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectQuery(sqlStmt).
			WithArgs("creator-1").
			WillReturnError(errors.New("db down"))

		_, err := repo.GetByID(context.Background(), "creator-1")
		require.ErrorContains(t, err, "db down")
	})
}

func TestCreatorRepository_DeleteForTests(t *testing.T) {
	t.Parallel()

	const sqlStmt = "DELETE FROM creators WHERE id = $1"

	t.Run("success returns nil", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("creator-1").
			WillReturnResult(pgconn.NewCommandTag("DELETE 1"))

		require.NoError(t, repo.DeleteForTests(context.Background(), "creator-1"))
	})

	t.Run("missing returns sql.ErrNoRows", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("missing").
			WillReturnResult(pgconn.NewCommandTag("DELETE 0"))

		err := repo.DeleteForTests(context.Background(), "missing")
		require.ErrorIs(t, err, sql.ErrNoRows)
	})

	t.Run("propagates db error", func(t *testing.T) {
		t.Parallel()
		mock := newPgxmock(t)
		repo := &creatorRepository{db: mock}

		mock.ExpectExec(sqlStmt).
			WithArgs("creator-1").
			WillReturnError(errors.New("db down"))

		err := repo.DeleteForTests(context.Background(), "creator-1")
		require.ErrorContains(t, err, "db down")
	})
}

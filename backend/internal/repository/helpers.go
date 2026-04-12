package repository

import (
	"slices"
	"strings"

	"github.com/Masterminds/squirrel"
	"github.com/elgris/stom"
)

type tagMap string

const (
	tagSelect tagMap = "db"
	tagInsert tagMap = "insert"
)

// toMap converts a struct to a map using the given stom mapper.
func toMap(row any, st stom.ToMapper) map[string]any {
	m, err := st.ToMap(row)
	if err != nil {
		panic(err)
	}
	return m
}

// insertEntities adds row values to an INSERT query builder.
func insertEntities(qb squirrel.InsertBuilder, st stom.ToMapper, cols []string, row any) squirrel.InsertBuilder {
	m, err := st.ToMap(row)
	if err != nil {
		panic(err)
	}
	values := make([]any, len(cols))
	for i, c := range cols {
		values[i] = m[c]
	}
	return qb.Values(values...)
}

func sortColumns(columns []string) []string {
	slices.Sort(columns)
	return columns
}

// returningClause builds a "RETURNING col1, col2, ..." suffix from a column list.
func returningClause(cols []string) string {
	return "RETURNING " + strings.Join(cols, ", ")
}

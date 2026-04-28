package domain

import "errors"

// DictionaryType is the public name of a dictionary served by GET /dictionaries/{type}.
// Each value corresponds to a separate physical table behind the scenes; the
// mapping lives in the service layer (see service.dictionaryTables).
type DictionaryType string

const (
	DictionaryTypeCategories DictionaryType = "categories"
	DictionaryTypeCities     DictionaryType = "cities"
)

// DictionaryTypeValues lists every dictionary the public API exposes.
// Used by tests for coverage and by the service to validate the inbound type.
var DictionaryTypeValues = []DictionaryType{
	DictionaryTypeCategories,
	DictionaryTypeCities,
}

// DictionaryEntry is one row served to the client — code is the stable
// identifier the client sends back, name is the label shown to the user,
// SortOrder controls the order in the UI. Fields shared across every
// dictionary on purpose: extra per-dictionary metadata stays in the dedicated
// domain types (Category / City) once it is needed.
type DictionaryEntry struct {
	Code      string
	Name      string
	SortOrder int
}

// ErrDictionaryUnknownType is the sentinel the service returns for a
// dictionary type the catalogue does not recognise. Handler maps it to 404.
var ErrDictionaryUnknownType = errors.New("unknown dictionary type")

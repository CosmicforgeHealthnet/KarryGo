// Package filters implements allowlisted filter and sort parsing.
//
// Unknown filters, unknown sort fields and malformed values are rejected.
// Raw client values are never concatenated into SQL; parsed values are only
// ever bound as query parameters. Tenant filtering is applied by repositories
// independently of anything parsed here.
package filters

import (
	"fmt"
	"net/url"
	"sort"
	"strings"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Kind constrains how a filter value is validated.
type Kind int

const (
	// KindString accepts any non-empty string up to 200 characters.
	KindString Kind = iota
	// KindEnum accepts only one of the declared values.
	KindEnum
)

// Field declares one allowlisted filter.
type Field struct {
	Kind   Kind
	Values []string // for KindEnum
}

// Schema declares the allowlisted filters and sortable fields of a resource.
type Schema struct {
	Fields map[string]Field
	Sorts  []string
}

// Parsed is the validated result of parsing query values against a schema.
type Parsed struct {
	Values    map[string]string
	SortField string
	SortDesc  bool
}

// Reserved query parameters that are not treated as filters.
var reserved = map[string]bool{"cursor": true, "limit": true, "sort": true, "page": true}

// Parse validates the query against the schema. It fails on any unknown
// filter, unknown sort field or malformed value.
func Parse(query url.Values, schema Schema) (Parsed, error) {
	parsed := Parsed{Values: map[string]string{}}
	var violations []apperrors.FieldViolation

	keys := make([]string, 0, len(query))
	for key := range query {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		if reserved[key] {
			continue
		}
		field, ok := schema.Fields[key]
		if !ok {
			violations = append(violations, apperrors.FieldViolation{Field: key, Message: "Unknown filter."})
			continue
		}
		value := query.Get(key)
		if err := validateValue(field, value); err != nil {
			violations = append(violations, apperrors.FieldViolation{Field: key, Message: err.Error()})
			continue
		}
		parsed.Values[key] = value
	}

	if rawSort := query.Get("sort"); rawSort != "" {
		field := strings.TrimPrefix(rawSort, "-")
		if !contains(schema.Sorts, field) {
			violations = append(violations, apperrors.FieldViolation{Field: "sort", Message: "Unknown sort field."})
		} else {
			parsed.SortField = field
			parsed.SortDesc = strings.HasPrefix(rawSort, "-")
		}
	}

	if len(violations) > 0 {
		return Parsed{}, apperrors.Validation("One or more filters are invalid.", violations)
	}
	return parsed, nil
}

func validateValue(field Field, value string) error {
	if value == "" || len(value) > 200 {
		return fmt.Errorf("Value is malformed.")
	}
	if field.Kind == KindEnum && !contains(field.Values, value) {
		return fmt.Errorf("Value is not allowed.")
	}
	return nil
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

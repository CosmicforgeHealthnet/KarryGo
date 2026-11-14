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

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/pagination"
)

// Kind constrains how a filter value is validated.
type Kind int

const (
	// KindString accepts any non-empty string up to 200 characters.
	KindString Kind = iota
	// KindEnum accepts only one of the declared values.
	KindEnum
	// KindBool accepts "true" or "false".
	KindBool
	// KindLimit accepts a page size (see platform/pagination).
	KindLimit
	// KindOffset accepts an offset position (see platform/pagination).
	KindOffset
	// KindCursor accepts an opaque cursor token; its content is validated by
	// the route that decodes it.
	KindCursor
)

// maxCursorLength bounds an opaque cursor token before it is decoded.
const maxCursorLength = 1024

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

// Validate is the strict form used by list routes. Unlike Parse it has no
// implicit reserved parameters: cursor, limit, offset and page are accepted
// only when the schema declares them, so a route that does not paginate by
// offset rejects "offset" instead of silently ignoring it. "sort" is accepted
// only when the schema declares sortable fields.
//
// An empty value means "not supplied", matching how every handler already
// treats it. A parameter repeated with several values is rejected, because
// handlers read only the first and the caller's intent is ambiguous.
func Validate(query url.Values, schema Schema) error {
	keys := make([]string, 0, len(query))
	for key := range query {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var violations []apperrors.FieldViolation
	for _, key := range keys {
		values := query[key]
		if key == "sort" && len(schema.Sorts) > 0 {
			if len(values) > 1 {
				violations = append(violations, apperrors.FieldViolation{Field: key, Message: "Specify this parameter once."})
			} else if raw := values[0]; raw != "" && !contains(schema.Sorts, strings.TrimPrefix(raw, "-")) {
				violations = append(violations, apperrors.FieldViolation{Field: key, Message: "Unknown sort field."})
			}
			continue
		}
		field, ok := schema.Fields[key]
		if !ok {
			violations = append(violations, apperrors.FieldViolation{Field: key, Message: "Unknown filter."})
			continue
		}
		if len(values) > 1 {
			violations = append(violations, apperrors.FieldViolation{Field: key, Message: "Specify this parameter once."})
			continue
		}
		if msg := strictValueProblem(field, values[0]); msg != "" {
			violations = append(violations, apperrors.FieldViolation{Field: key, Message: msg})
		}
	}
	if len(violations) > 0 {
		return apperrors.Validation("One or more query parameters are not supported.", violations)
	}
	return nil
}

func strictValueProblem(field Field, value string) string {
	if value == "" {
		return ""
	}
	switch field.Kind {
	case KindLimit:
		if !pagination.ValidLimit(value) {
			return fmt.Sprintf("Must be a whole number from 1 to %d.", pagination.MaxLimit)
		}
	case KindOffset:
		if !pagination.ValidOffset(value) {
			return fmt.Sprintf("Must be a whole number from 0 to %d.", pagination.MaxOffset)
		}
	case KindCursor:
		if len(value) > maxCursorLength {
			return "Value is malformed."
		}
	case KindBool:
		if value != "true" && value != "false" {
			return "Must be true or false."
		}
	case KindEnum:
		if !contains(field.Values, value) {
			return "Value is not allowed."
		}
	default:
		if len(value) > 200 {
			return "Value is malformed."
		}
	}
	return ""
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}

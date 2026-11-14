// Package pagination holds the page-size and offset rules every list route
// shares (spec 6 and 36: cursor preferred, page/limit kept where already
// published, maximum 200, stable order with an id tie-breaker).
//
// Two layers use it:
//
//   - HTTP handlers call Parse, which turns a malformed, negative or oversized
//     value into a validation error (HTTP 422) instead of letting it reach SQL.
//     PostgreSQL rejects a negative OFFSET, so an unvalidated "offset=-1" used
//     to surface as HTTP 500.
//   - Services call ClampLimit/ClampOffset right before binding the values, so
//     a caller that bypasses HTTP (a worker, a test, a future handler) can never
//     produce an invalid query either.
package pagination

import (
	"strconv"
	"strings"

	"cosmicforge/logistics/shared/go/apperrors"
)

const (
	// DefaultLimit is the page size used when the caller does not ask for one.
	DefaultLimit = 50
	// MaxLimit is the largest page any list route serves (spec 36).
	MaxLimit = 200
	// MaxOffset bounds offset pagination. Offset lists exist only for
	// backward compatibility; deeper positions make PostgreSQL scan and discard
	// every skipped row, so callers past this point must narrow with filters.
	MaxOffset = 10000
)

// Page is a validated offset page.
type Page struct {
	Limit  int
	Offset int
}

// Parse validates raw limit/offset query values. An empty value takes the
// default (limit DefaultLimit, offset 0). Anything that is not a whole number,
// is negative, or exceeds MaxLimit/MaxOffset is a validation error naming the
// offending field. A limit of 0 means "the default", as it always has.
func Parse(rawLimit, rawOffset string) (Page, error) {
	var violations []apperrors.FieldViolation
	limit, ok := parseBounded(rawLimit, MaxLimit)
	if !ok {
		violations = append(violations, apperrors.FieldViolation{Field: "limit", Message: "Must be a whole number from 1 to " + strconv.Itoa(MaxLimit) + "."})
	}
	offset, ok := parseBounded(rawOffset, MaxOffset)
	if !ok {
		violations = append(violations, apperrors.FieldViolation{Field: "offset", Message: "Must be a whole number from 0 to " + strconv.Itoa(MaxOffset) + "."})
	}
	if len(violations) > 0 {
		return Page{}, apperrors.Validation("The page parameters are invalid.", violations)
	}
	return Page{Limit: ClampLimit(limit), Offset: offset}, nil
}

// ValidLimit reports whether a raw limit value is acceptable to Parse.
func ValidLimit(raw string) bool {
	_, ok := parseBounded(raw, MaxLimit)
	return ok
}

// ValidOffset reports whether a raw offset value is acceptable to Parse.
func ValidOffset(raw string) bool {
	_, ok := parseBounded(raw, MaxOffset)
	return ok
}

func parseBounded(raw string, max int) (int, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, true
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 || n > max {
		return 0, false
	}
	return n, true
}

// ClampLimit normalizes a page size into [1, MaxLimit], using DefaultLimit for
// zero or negative values.
func ClampLimit(n int) int {
	if n <= 0 {
		return DefaultLimit
	}
	if n > MaxLimit {
		return MaxLimit
	}
	return n
}

// ClampOffset normalizes an offset into [0, MaxOffset].
func ClampOffset(n int) int {
	if n < 0 {
		return 0
	}
	if n > MaxOffset {
		return MaxOffset
	}
	return n
}

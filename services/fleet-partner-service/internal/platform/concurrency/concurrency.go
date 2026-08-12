// Package concurrency implements the optimistic-concurrency foundation.
//
// Aggregates carry an integer version. Mutating commands must supply the
// expected version through If-Match or an explicit expectedVersion field.
// Repositories perform compare-and-swap updates (WHERE version = $expected)
// and surface ErrVersionMismatch when the row moved on.
package concurrency

import (
	"errors"
	"strconv"
	"strings"

	"cosmicforge/logistics/shared/go/apperrors"
)

// ErrVersionMismatch is returned by repositories when a compare-and-swap
// update matched zero rows because the stored version differs.
var ErrVersionMismatch = errors.New("concurrency: expected version does not match")

// ParseIfMatch parses an If-Match header value of the form `"3"` or `3`.
func ParseIfMatch(header string) (int64, error) {
	trimmed := strings.TrimSpace(header)
	trimmed = strings.Trim(trimmed, `"`)
	if trimmed == "" {
		return 0, apperrors.BadRequest("If-Match header is required for this operation.", nil)
	}
	version, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || version < 1 {
		return 0, apperrors.BadRequest("If-Match header must contain a positive version number.", nil)
	}
	return version, nil
}

// ExpectedVersion resolves the expected version from an If-Match header and
// an optional body field; exactly one source must provide it.
func ExpectedVersion(ifMatch string, bodyVersion *int64) (int64, error) {
	if ifMatch != "" {
		return ParseIfMatch(ifMatch)
	}
	if bodyVersion != nil {
		if *bodyVersion < 1 {
			return 0, apperrors.BadRequest("expectedVersion must be a positive version number.", nil)
		}
		return *bodyVersion, nil
	}
	return 0, apperrors.BadRequest("Provide the expected version via If-Match or expectedVersion.", nil)
}

// AsConflict maps ErrVersionMismatch onto the public conflict error; any
// other error passes through unchanged.
func AsConflict(err error) error {
	if errors.Is(err, ErrVersionMismatch) {
		return apperrors.Conflict("The resource was modified by someone else. Reload and retry.", err)
	}
	return err
}

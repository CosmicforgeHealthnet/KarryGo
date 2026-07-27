// Package cursor implements opaque cursor pagination primitives.
//
// Cursors encode a stable sort key plus a deterministic ID tie-breaker. They
// are opaque base64url tokens; malformed cursors are rejected with a
// validation error rather than silently reset.
package cursor

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// DefaultLimit and MaxLimit bound page sizes for cursor-paginated resources.
const (
	DefaultLimit = 25
	MaxLimit     = 100
)

// Cursor is the decoded position of the last item on the previous page.
type Cursor struct {
	// SortValue is the RFC 3339 (nanosecond) representation of the primary
	// sort key of the last seen row.
	SortValue string `json:"s"`
	// LastID is the opaque ID tie-breaker of the last seen row.
	LastID string `json:"id"`
}

// Encode serializes the cursor to an opaque token.
func Encode(c Cursor) string {
	raw, _ := json.Marshal(c)
	return base64.RawURLEncoding.EncodeToString(raw)
}

// EncodeTime builds a cursor from a timestamp sort key and ID tie-breaker.
func EncodeTime(sortedAt time.Time, lastID string) string {
	return Encode(Cursor{SortValue: sortedAt.UTC().Format(time.RFC3339Nano), LastID: lastID})
}

// Decode parses an opaque cursor token. An empty token returns a zero cursor
// with ok=false, meaning "first page".
func Decode(token string) (Cursor, bool, error) {
	if token == "" {
		return Cursor{}, false, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return Cursor{}, false, fmt.Errorf("cursor: malformed token")
	}
	var c Cursor
	if err := json.Unmarshal(raw, &c); err != nil {
		return Cursor{}, false, fmt.Errorf("cursor: malformed token")
	}
	if c.SortValue == "" || c.LastID == "" {
		return Cursor{}, false, fmt.Errorf("cursor: malformed token")
	}
	if _, err := time.Parse(time.RFC3339Nano, c.SortValue); err != nil {
		return Cursor{}, false, fmt.Errorf("cursor: malformed token")
	}
	return c, true, nil
}

// SortTime returns the parsed time sort key. Only valid after a successful Decode.
func (c Cursor) SortTime() time.Time {
	t, _ := time.Parse(time.RFC3339Nano, c.SortValue)
	return t
}

// ClampLimit normalizes a requested page size into [1, MaxLimit].
func ClampLimit(requested int) int {
	if requested < 1 {
		return DefaultLimit
	}
	if requested > MaxLimit {
		return MaxLimit
	}
	return requested
}

package infra

import (
	"strconv"

	"github.com/google/uuid"
)

// uuidClause returns an extra OR clause matching the id column only when idOrRef
// is a valid UUID. Without this guard, Postgres would fail casting a booking_ref
// like "KGP-A1B2C3D4" to the uuid id column.
func uuidClause(idOrRef string) string {
	if _, err := uuid.Parse(idOrRef); err == nil {
		return " OR id = $2"
	}
	return ""
}

func itoa(n int) string { return strconv.Itoa(n) }

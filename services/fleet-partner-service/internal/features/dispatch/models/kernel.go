// Package models holds the Section 16 dispatch domain -- dispatch plans,
// explainable assignment candidates, geofences, operational alerts, rescue
// cases and the control-tower board -- plus the small kernel the dispatch,
// routes and tracking features share: feature flags, the list filter, the
// not-found/conflict sentinels and the capability gate.
package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// ErrNotFound and ErrConflict are the Section 16 persistence sentinels. A
// repository returns ErrConflict when an expected version or idempotency
// fingerprint does not match.
var (
	ErrNotFound = errors.New("section16: not found")
	ErrConflict = errors.New("section16: conflict")
)

// Flags switches Section 16 capabilities. One value configures the
// dispatch, routes and tracking features together.
type Flags struct {
	DispatchPlanning, RoutePlanning, RouteOptimization, RoutePublication          bool
	ControlTower, TrackingProjections, Geofences, OperationalAlerts               bool
	Workers, KillSwitch, OptimizationProviderConfigured, TrackingSourceConfigured bool
}

// Gate answers a disabled capability: everything is unavailable under the
// kill switch; a disabled write is forbidden and a disabled read is the
// safe not-found.
func (f Flags) Gate(enabled, write bool, code string) error {
	if f.KillSwitch {
		return errorcodes.Unavailable("fleet_operations_disabled", "Fleet operations are disabled.")
	}
	if !enabled {
		if write {
			return errorcodes.Forbidden(apperrors.Code(code), "This capability is disabled.")
		}
		return errorcodes.NotFound(apperrors.Code(code), "Resource not found.")
	}
	return nil
}

// PageFilter is the shared Section 16 list filter. A list pages either by
// Offset (kept for compatibility) or by the keyset position After/AfterID of
// the last row of the previous page, never both.
type PageFilter struct {
	BranchID, Status, Service, ProviderID, VehicleReference, AlertType string
	Date                                                               *time.Time
	Limit, Offset                                                      int
	After                                                              *time.Time
	AfterID                                                            string
}

// BoundedLimit clamps a page size to 1..200 with a default of 50.
func BoundedLimit(n int) int {
	if n <= 0 {
		return 50
	}
	if n > 200 {
		return 200
	}
	return n
}

// Fingerprint is the deterministic request fingerprint used for
// idempotency-key reuse detection.
func Fingerprint(v any) string {
	raw, _ := json.Marshal(v)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// RequireVersion rejects a write without an expected version.
func RequireVersion(v int64) error {
	if v <= 0 {
		return apperrors.BadRequest("expectedVersion or If-Match is required.", nil)
	}
	return nil
}

// NotFound is the safe Section 16 not-found answer for an object the caller
// cannot see.
func NotFound() error { return errorcodes.NotFound("section16_not_found", "Resource not found.") }

// RequireBranch admits a non-empty branch inside the caller's scope.
func RequireBranch(t tenant.Context, b string) error {
	if b == "" || !t.BranchPermitted(b) {
		return NotFound()
	}
	return nil
}

// TrimPage cuts a size+1 fetch down to one page and returns the opaque
// cursor of the next page, or "" when the fetch did not exceed size. key
// returns a row's sort time and id tie-breaker. The cursor is taken from the
// last row kept before any branch filtering, so filtering a page never
// skips or repeats rows on the next one.
func TrimPage[T any](rows []T, size int, key func(T) (time.Time, string)) ([]T, string) {
	if len(rows) <= size {
		return rows, ""
	}
	rows = rows[:size]
	at, id := key(rows[size-1])
	return rows, cursor.EncodeTime(at, id)
}

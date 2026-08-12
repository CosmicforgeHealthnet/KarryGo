// Package serviceauth implements Section 28's service-to-service credential
// lifecycle and route/method/event scoping, layered strictly on top of the
// existing shared/go/serviceauth HMAC signature primitives (never modified
// -- that package is used by every other service in the platform; hardening
// it centrally is a separate, platform-wide decision outside this service's
// scope, the same reasoning internal/platform/noncestore already documents
// for the identical constraint on replay protection).
//
// Today every internal-route handler group (documents, finance, marketplace,
// backgroundops) enforces its OWN caller allowlist independently
// (FLEET_*_ALLOWED_CALLERS), with no route/method granularity: a caller in
// the allowlist may call EVERY route that handler group registers, forever,
// with no expiry or revocation short of an operator editing an env var and
// redeploying. This package adds ONE closed, versioned, auditable, rotatable
// declaration of what a caller may actually do (which routes, which
// methods, which event types, for how long), checked centrally by
// internal/middleware.ServiceScopeAuth rather than scattered per-handler --
// existing per-handler allowlists are left in place as defense-in-depth,
// not replaced, so no existing caller's access can ever narrow by surprise
// from this package alone.
//
// A Credential row is pure metadata. The actual HMAC secret bytes never
// appear here or in the database at all -- see secrets.go's doc comment.
package serviceauth

import (
	"strings"
	"time"
)

// Wildcard is the one closed-registry-sanctioned token meaning "any method"/
// "any route pattern"/"any event type" for a dimension. It is matched as an
// exact list membership check, like every other entry -- never a substring
// or prefix match against the request.
const Wildcard = "*"

// Credential is one versioned, scoped service-to-service credential's
// metadata -- never its secret. AllowedRoutePatterns entries are route
// identifiers exactly as internal/platform/riskregistry.Entry.Path encodes
// them (the gin route TEMPLATE, e.g. "/internal/fleet-partner/v1/documents/
// media-events" -- never a resolved path with real IDs substituted), the
// same closed table this service's rate-limit classification already keeps
// in sync with what is actually mounted.
type Credential struct {
	ID                   string
	ServiceName          string
	Version              int
	AllowedMethods       []string
	AllowedRoutePatterns []string
	AllowedEventTypes    []string
	ActiveFrom           time.Time
	ExpiresAt            *time.Time
	RevokedAt            *time.Time
	RevokedReason        string
	LastUsedAt           *time.Time
	CreatedBy            string
	VersionNote          string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// IsActive reports whether the credential is currently usable: not revoked,
// its active-from has passed, and (if set) its expiry has not.
func (c Credential) IsActive(now time.Time) bool {
	if c.RevokedAt != nil {
		return false
	}
	if now.Before(c.ActiveFrom) {
		return false
	}
	if c.ExpiresAt != nil && !now.Before(*c.ExpiresAt) {
		return false
	}
	return true
}

// AllowsMethod reports whether method is within this credential's declared
// method scope.
func (c Credential) AllowsMethod(method string) bool {
	return matches(c.AllowedMethods, strings.ToUpper(method))
}

// AllowsRoute reports whether routePattern (a gin route template, not a
// resolved path) is within this credential's declared route scope.
func (c Credential) AllowsRoute(routePattern string) bool {
	return matches(c.AllowedRoutePatterns, routePattern)
}

// AllowsEventType reports whether eventType is within this credential's
// declared event-type scope. An empty eventType (the route is not
// event-carrying) always passes -- there is nothing to scope.
func (c Credential) AllowsEventType(eventType string) bool {
	if eventType == "" {
		return true
	}
	return matches(c.AllowedEventTypes, eventType)
}

// matches reports whether value is exactly present in list, or list
// contains the closed Wildcard token. Never substring/prefix matching, per
// Section 28's explicit requirement.
func matches(list []string, value string) bool {
	for _, entry := range list {
		if entry == Wildcard || entry == value {
			return true
		}
	}
	return false
}

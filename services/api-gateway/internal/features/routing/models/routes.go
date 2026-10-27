// Package models holds the Section 18 edge route table.
//
// The gateway routes; it does not decide. Every group below is forwarded to
// the service that owns it, and that service authenticates and authorizes
// the request exactly as it does when called directly. The gateway adds
// correlation, timeouts and a circuit breaker, and it refuses to forward
// anything internal.
package models

import (
	"net/url"
	"path"
	"sort"
	"strings"
)

// Scope is the audience a route group serves. It shapes edge concerns --
// rate-limit keys, the X-Gateway-Scope hint -- never authorization.
type Scope string

const (
	ScopePublic   Scope = "PUBLIC"
	ScopeCustomer Scope = "CUSTOMER"
	ScopeProvider Scope = "PROVIDER"
	ScopeVendor   Scope = "VENDOR"
	ScopeFleet    Scope = "FLEET"
	ScopePartner  Scope = "PARTNER"
	ScopeAdmin    Scope = "ADMIN"
	ScopeMixed    Scope = "MIXED"
)

// Upstream names. Each maps to one GATEWAY_UPSTREAM_<NAME> origin.
const (
	UpstreamCustomer     = "customer"
	UpstreamTaxi         = "taxi"
	UpstreamDispatch     = "dispatch"
	UpstreamHauling      = "hauling"
	UpstreamWallet       = "payment_wallet"
	UpstreamNotification = "notification"
	UpstreamSupport      = "support"
	UpstreamVendor       = "vendor"
	UpstreamFleet        = "fleet"
	UpstreamPartnerWeb   = "partner_web"
	UpstreamAdmin        = "admin"
	UpstreamVerification = "verification"
	UpstreamReferral     = "referral"
)

// Route is one forwarded prefix.
type Route struct {
	Prefix   string
	Upstream string
	Scope    Scope
}

// DefaultRoutes is the platform's public surface, keyed by each owning
// service's own API base. Where two services share a base -- Dispatch serves
// customers under /api/v1/customer/dispatch and a notification proxy under
// /api/v1/customer/notifications, inside customer-service's /api/v1/customer
// -- the longer prefix wins.
//
// Deliberately absent: media-file-service (uploads are internal-service
// only) and analytics-service (every route is service-signed).
func DefaultRoutes() []Route {
	return []Route{
		{"/api/v1/customer/dispatch", UpstreamDispatch, ScopeCustomer},
		{"/api/v1/customer/notifications", UpstreamDispatch, ScopeCustomer},
		{"/api/v1/customer", UpstreamCustomer, ScopeCustomer},
		{"/api/v1/provider", UpstreamDispatch, ScopeProvider},
		{"/api/v1/auth", UpstreamDispatch, ScopeProvider},
		{"/api/v1/dispatch", UpstreamDispatch, ScopeMixed},
		{"/api/v1/taxi", UpstreamTaxi, ScopeMixed},
		{"/api/v1/hauling", UpstreamHauling, ScopeMixed},
		{"/api/v1/payment-wallet", UpstreamWallet, ScopeMixed},
		{"/api/v1/notifications/ws", UpstreamNotification, ScopePublic},
		{"/api/v1/support-disputes", UpstreamSupport, ScopeMixed},
		// Referral codes and rewards for customers and drivers alike; each is
		// authenticated by referral-service with its own app's token.
		{"/api/v1/referrals", UpstreamReferral, ScopeMixed},
		{"/api/v1/vendor", UpstreamVendor, ScopeVendor},
		{"/api/v1/commerce", UpstreamVendor, ScopeCustomer},
		{"/api/v1/partner/v1", UpstreamPartnerWeb, ScopePartner},
		{"/fleet-partner/", UpstreamFleet, ScopeFleet},
	}
}

// Table matches paths to routes, longest prefix first.
type Table struct{ routes []Route }

// NewTable builds a table.
func NewTable(routes []Route) Table {
	sorted := append([]Route(nil), routes...)
	sort.SliceStable(sorted, func(i, j int) bool { return len(sorted[i].Prefix) > len(sorted[j].Prefix) })
	return Table{routes: sorted}
}

// Match finds the route for a path. A prefix matches only at a segment
// boundary: /api/v1/taxi must not capture /api/v1/taxicab.
func (t Table) Match(path string) (Route, bool) {
	if IsInternalPath(path) {
		return Route{}, false
	}
	for _, route := range t.routes {
		prefix := strings.TrimSuffix(route.Prefix, "/")
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return route, true
		}
	}
	return Route{}, false
}

// Upstreams lists the upstream names the table uses.
func (t Table) Upstreams() []string {
	seen := map[string]bool{}
	var names []string
	for _, route := range t.routes {
		if !seen[route.Upstream] {
			seen[route.Upstream] = true
			names = append(names, route.Upstream)
		}
	}
	sort.Strings(names)
	return names
}

// IsInternalPath reports a path the edge must never forward.
//
// Service-to-service routes live under an "internal" segment everywhere on
// the platform (/internal/..., /api/v1/internal/..., /api/v1/<svc>/internal/,
// /internal/fleet-partner/...). They are HMAC-protected, but the gateway does
// not rely on that: it will not carry a request to one at all. The check is
// on the decoded, cleaned path, case-insensitively, so encoding and dot
// segments cannot smuggle one through.
func IsInternalPath(value string) bool {
	// Inspect every encoding layer: a downstream server may decode again.
	for depth := 0; depth < 8; depth++ {
		if privatePath(value) {
			return true
		}
		decoded, err := url.PathUnescape(value)
		if err != nil {
			return true
		}
		if decoded == value {
			return false
		}
		value = decoded
	}
	return true // reject excessive encoding instead of guessing downstream behavior
}

func privatePath(value string) bool {
	value = strings.ReplaceAll(value, "\\", "/")
	for _, segment := range strings.Split(value, "/") {
		if segment == ".." {
			return true
		}
	}
	value = path.Clean("/" + strings.TrimLeft(value, "/"))
	// These mounted routes use service credentials despite having no
	// "internal" segment. Auth middleware, not spelling, defines exposure.
	lower := strings.ToLower(value)
	for _, prefix := range []string{"/api/v1/admin", "/api/v1/support-disputes/admin", "/api/v1/verification-compliance"} {
		if lower == prefix || strings.HasPrefix(lower, prefix+"/") {
			return true
		}
	}
	// Operational scrape endpoints (/api/v1/<svc>/metrics, .../outbox-metrics)
	// are secured by network placement, not by caller auth: Taxi's booking
	// metrics carry no auth at all. They are never a public route.
	if segments := strings.Split(strings.Trim(lower, "/"), "/"); len(segments) == 4 &&
		segments[0] == "api" && segments[1] == "v1" &&
		(segments[3] == "metrics" || segments[3] == "outbox-metrics") {
		return true
	}
	if lower == "/api/v1/notifications" || strings.HasPrefix(lower, "/api/v1/notifications/") {
		if lower != "/api/v1/notifications/ws" {
			return true
		}
	}
	for _, segment := range strings.Split(lower, "/") {
		if segment == "internal" || segment == ".." || strings.HasPrefix(segment, "internal%") {
			return true
		}
	}
	return false
}

// WebsocketEndpoint is one public path that may switch protocols at the edge.
//
// Every other path answers an upgrade attempt with 404 before any upstream is
// contacted. Pattern segments are literal except "{id}", which matches one
// opaque identifier segment.
type WebsocketEndpoint struct {
	Pattern  string
	Upstream string
}

// PublicWebsocketEndpoints are the websocket endpoints the apps actually use
// through the edge:
//
//   - notification-service realtime feed (customer, truck and dispatch apps);
//   - customer live tracking of a dispatch booking (dispatch_api.dart);
//   - customer live tracking of a hauling booking (hauling_api.dart).
//
// Dispatch's rider-side /ws/provider/:id/location is not under any public
// route prefix and is deliberately absent.
func PublicWebsocketEndpoints() []WebsocketEndpoint {
	return []WebsocketEndpoint{
		{"/api/v1/notifications/ws", UpstreamNotification},
		{"/api/v1/customer/dispatch/bookings/{id}/track", UpstreamDispatch},
		{"/api/v1/hauling/customer/bookings/{id}/track", UpstreamHauling},
	}
}

// WebsocketEndpointFor returns the allowlisted endpoint a path names. The
// match is exact and case-sensitive on the decoded path: no trailing slash,
// no empty or dot segments, and an identifier made only of [A-Za-z0-9_-].
func WebsocketEndpointFor(value string) (WebsocketEndpoint, bool) {
	if value == "" || IsInternalPath(value) {
		return WebsocketEndpoint{}, false
	}
	got := strings.Split(value, "/")
	for _, endpoint := range PublicWebsocketEndpoints() {
		want := strings.Split(endpoint.Pattern, "/")
		if len(got) != len(want) {
			continue
		}
		matched := true
		for i := range want {
			if want[i] == "{id}" {
				if !opaqueID(got[i]) {
					matched = false
					break
				}
				continue
			}
			if got[i] != want[i] {
				matched = false
				break
			}
		}
		if matched {
			return endpoint, true
		}
	}
	return WebsocketEndpoint{}, false
}

func opaqueID(segment string) bool {
	if segment == "" || len(segment) > 128 {
		return false
	}
	for _, r := range segment {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

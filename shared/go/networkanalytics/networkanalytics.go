// Package networkanalytics is the Section 13 contract between the logistics
// services and analytics-service.
//
// Analytics is ADVISORY. It counts what the owning services already decided;
// it never decides anything itself, and nothing may depend on it being up. A
// reader that cannot reach it gets a neutral answer, never an error that
// blocks a booking, a match or an order.
//
// Producers do not adopt a new vocabulary to feed it. Each service keeps
// emitting the event names it already emits -- taxi.recovery.started,
// hauling.capability_mismatch, dispatch.capability_mismatch.v1, trip.completed.v1
// -- and Classify maps each one to the fact analytics projects. A second,
// "network.*" dialect for facts that already have names would leave every
// other consumer of those events having to know which one a given event
// speaks.
package networkanalytics

import (
	"strings"
)

// IngestPath is analytics-service's network event ingress. It takes the
// platform's canonical shared/go/events.Envelope.
const IngestPath = "/internal/analytics/v1/network-events"

// Authorities are the logistics services whose work is measured.
const (
	AuthorityTaxi     = "TAXI"
	AuthorityDispatch = "DISPATCH"
	AuthorityHauling  = "HAULING"
	AuthorityVendor   = "VENDOR"
)

// ValidAuthority reports whether an authority is one analytics measures.
func ValidAuthority(authority string) bool {
	switch authority {
	case AuthorityTaxi, AuthorityDispatch, AuthorityHauling:
		return true
	}
	return false
}

// Kind is the fact an event contributes to the read models.
type Kind string

const (
	KindRecoveryStarted    Kind = "RECOVERY_STARTED"
	KindRecoveryReplaced   Kind = "RECOVERY_PROVIDER_REPLACED"
	KindRecoveryResolved   Kind = "RECOVERY_RESOLVED"
	KindRecoveryFailed     Kind = "RECOVERY_FAILED"
	KindGuaranteeBreached  Kind = "GUARANTEE_BREACHED"
	KindCapabilityMismatch Kind = "CAPABILITY_MISMATCH"
	KindProviderArrived    Kind = "PROVIDER_ARRIVED"
	KindProviderDeclined   Kind = "PROVIDER_DECLINED"
	KindMissionCompleted   Kind = "MISSION_COMPLETED"
	KindMissionFailed      Kind = "MISSION_FAILED"
	KindMissionCancelled   Kind = "MISSION_CANCELLED"
	KindSupplySnapshot     Kind = "SUPPLY_SNAPSHOT"
	KindDemandObserved     Kind = "DEMAND_OBSERVED"
	KindBranchReadiness    Kind = "BRANCH_READINESS"
)

// Snapshot kinds describe CURRENT state rather than something that happened,
// so an older version must never overwrite a newer one.
func (k Kind) Snapshot() bool { return k == KindSupplySnapshot }

// Canonical event names for the few facts no service emitted before Section 13.
const (
	EventSupplySnapshot  = "network.supply.snapshot.v1"
	EventDemandObserved  = "network.demand.observed.v1"
	EventBranchReadiness = "vendor.branch.readiness_observed.v1"
)

type classification struct {
	authority string
	kind      Kind
}

// eventKinds is every producer event name analytics understands, under the
// name the producer actually uses. Where a service emits both an unversioned
// and a ".v1" spelling (Taxi passenger vs Taxi Goods), both are listed.
var eventKinds = map[string]classification{
	// Taxi -- passenger and goods.
	"taxi.recovery.started":              {AuthorityTaxi, KindRecoveryStarted},
	"taxi.recovery.started.v1":           {AuthorityTaxi, KindRecoveryStarted},
	"taxi.recovery.provider_replaced":    {AuthorityTaxi, KindRecoveryReplaced},
	"taxi.recovery.provider_replaced.v1": {AuthorityTaxi, KindRecoveryReplaced},
	"taxi.recovery.resolved":             {AuthorityTaxi, KindRecoveryResolved},
	"taxi.recovery.resolved.v1":          {AuthorityTaxi, KindRecoveryResolved},
	"taxi.recovery.failed":               {AuthorityTaxi, KindRecoveryFailed},
	"taxi.recovery.failed.v1":            {AuthorityTaxi, KindRecoveryFailed},
	"taxi.guarantee.breached":            {AuthorityTaxi, KindGuaranteeBreached},
	"taxi.guarantee.breached.v1":         {AuthorityTaxi, KindGuaranteeBreached},
	"taxi.booking.arrived_at_pickup":     {AuthorityTaxi, KindProviderArrived},
	"taxi.goods.pickup.arrived.v1":       {AuthorityTaxi, KindProviderArrived},
	"taxi.goods.completed.v1":            {AuthorityTaxi, KindMissionCompleted},
	"taxi.goods.mission.cancelled.v1":    {AuthorityTaxi, KindMissionCancelled},

	// Hauling.
	"hauling.recovery.started":           {AuthorityHauling, KindRecoveryStarted},
	"hauling.recovery.provider_replaced": {AuthorityHauling, KindRecoveryReplaced},
	"hauling.recovery.resolved":          {AuthorityHauling, KindRecoveryResolved},
	"hauling.recovery.failed":            {AuthorityHauling, KindRecoveryFailed},
	"hauling.guarantee.breached":         {AuthorityHauling, KindGuaranteeBreached},
	"hauling.capability_mismatch":        {AuthorityHauling, KindCapabilityMismatch},

	// Dispatch. A delivery recovery being SELECTED is the start of recovery;
	// a provider replacement starting is the replacement itself.
	"dispatch.capability_mismatch.v1":          {AuthorityDispatch, KindCapabilityMismatch},
	"dispatch.delivery_recovery.selected.v1":   {AuthorityDispatch, KindRecoveryStarted},
	"dispatch.provider_replacement.started.v1": {AuthorityDispatch, KindRecoveryReplaced},
	"trip.provider_arrived":                    {AuthorityDispatch, KindProviderArrived},
	// A rider declining an offer, with its reason. Emitted by Dispatch inside
	// the decline transaction.
	"dispatch.offer.declined.v1": {AuthorityDispatch, KindProviderDeclined},
	"trip.completed.v1":          {AuthorityDispatch, KindMissionCompleted},
	"trip.delivery_failed.v1":    {AuthorityDispatch, KindMissionFailed},
	"trip.cancelled.v1":          {AuthorityDispatch, KindMissionCancelled},

	// Vendor.
	EventBranchReadiness: {AuthorityVendor, KindBranchReadiness},
}

// safetyDeclineReasons are declines a provider makes FOR safety. They are the
// decline doing its job -- a rider refusing an unsafe vehicle is the outcome
// the platform wants -- so they are counted apart from ordinary declines and
// never against the provider.
var safetyDeclineReasons = map[string]bool{
	"UNSAFE_VEHICLE": true, "VEHICLE_DEFECT": true, "CARGO_OR_SAFETY_CONCERN": true,
	"SECURITY_CONCERN": true, "UNSAFE_ROUTE": true, "UNSAFE_CARGO": true,
}

// IsSafetyDecline reports whether a decline reason is a safety refusal.
func IsSafetyDecline(reason string) bool {
	return safetyDeclineReasons[strings.ToUpper(strings.TrimSpace(reason))]
}

// Classify maps a producer's event name to the authority whose work it
// measures and the fact it contributes. Unknown names are not an error: the
// event is kept, just not projected.
//
// The network.* snapshot and demand events carry their authority in the
// payload, so their authority is resolved from there.
func Classify(eventName, payloadAuthority string) (authority string, kind Kind, ok bool) {
	name := strings.TrimSpace(eventName)
	switch name {
	case EventSupplySnapshot, EventDemandObserved:
		authority = strings.ToUpper(strings.TrimSpace(payloadAuthority))
		if !ValidAuthority(authority) {
			return "", "", false
		}
		if name == EventSupplySnapshot {
			return authority, KindSupplySnapshot, true
		}
		return authority, KindDemandObserved, true
	}
	found, ok := eventKinds[name]
	return found.authority, found.kind, ok
}

// KnownEventNames lists every classified producer name, for documentation and
// for producers deciding what to route to analytics.
func KnownEventNames() []string {
	names := make([]string, 0, len(eventKinds)+2)
	for name := range eventKinds {
		names = append(names, name)
	}
	return append(names, EventSupplySnapshot, EventDemandObserved)
}

// forbiddenTokens never survive ingestion. Analytics measures work, not
// people or money: a location, a contact detail, free text or an amount has no
// place in a network metric, and the surest way to keep one out of a read
// model is never to store it.
//
// Keys are compared by whole underscore-separated token, not by substring:
// "lat" must drop "pickup_lat" but not "late_seconds" or "platform".
var forbiddenTokens = map[string]bool{
	"lat": true, "lng": true, "lon": true, "latitude": true, "longitude": true,
	"coordinates": true, "location": true, "address": true, "polyline": true,
	"phone": true, "email": true, "name": true, "contact": true, "receiver": true,
	"sender": true, "description": true, "note": true, "notes": true, "comment": true,
	"amount": true, "fare": true, "price": true, "kobo": true, "minor": true, "fee": true,
	"commission": true, "payout": true, "wallet": true, "currency": true,
	"token": true, "secret": true, "otp": true, "pin": true, "hash": true, "credential": true,
}

// Minimize drops every payload key that could carry location, identity,
// contact, free text, money or credentials. Nested objects are minimized
// recursively; arrays are kept only when they hold plain values.
func Minimize(payload map[string]any) map[string]any {
	out := make(map[string]any, len(payload))
	for key, value := range payload {
		if !allowedKey(key) {
			continue
		}
		switch typed := value.(type) {
		case map[string]any:
			value = Minimize(typed)
		case []any:
			value = minimizeList(typed)
		}
		out[key] = value
	}
	return out
}

func minimizeList(values []any) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		if nested, isMap := value.(map[string]any); isMap {
			out = append(out, Minimize(nested))
			continue
		}
		out = append(out, value)
	}
	return out
}

func allowedKey(key string) bool {
	tokens := keyTokens(key)
	if len(tokens) == 0 {
		return false
	}
	for _, token := range tokens {
		if forbiddenTokens[token] {
			return false
		}
	}
	return true
}

// keyTokens splits a key into lower-case words across every convention the
// producers use: snake_case, kebab-case, dotted and camelCase. Lower-casing
// first would turn "pickupAddress" into one word and let it through.
func keyTokens(key string) []string {
	var tokens []string
	var current strings.Builder
	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
		}
	}
	runes := []rune(strings.TrimSpace(key))
	for i, r := range runes {
		switch {
		case r >= 'A' && r <= 'Z':
			// A capital starts a new word, unless it continues an acronym
			// ("ETAMinutes" -> "eta", "minutes").
			if i > 0 && (isLower(runes[i-1]) || isDigit(runes[i-1]) ||
				(i+1 < len(runes) && isLower(runes[i+1]) && isUpper(runes[i-1]))) {
				flush()
			}
			current.WriteRune(r + ('a' - 'A'))
		case isLower(r) || isDigit(r):
			current.WriteRune(r)
		default:
			flush()
		}
	}
	flush()
	return tokens
}

func isLower(r rune) bool { return r >= 'a' && r <= 'z' }
func isUpper(r rune) bool { return r >= 'A' && r <= 'Z' }
func isDigit(r rune) bool { return r >= '0' && r <= '9' }

// NormalizeKeys rewrites every key to snake_case, recursively, so the read
// models see one spelling whether a producer wrote "providerId" or
// "provider_id". Where two spellings of one key collide, the snake_case
// original wins.
func NormalizeKeys(payload map[string]any) map[string]any {
	out := make(map[string]any, len(payload))
	for key, value := range payload {
		switch typed := value.(type) {
		case map[string]any:
			value = NormalizeKeys(typed)
		case []any:
			list := make([]any, len(typed))
			for i, item := range typed {
				if nested, isMap := item.(map[string]any); isMap {
					item = NormalizeKeys(nested)
				}
				list[i] = item
			}
			value = list
		}
		snake := strings.Join(keyTokens(key), "_")
		if _, taken := out[snake]; taken && snake != key {
			continue
		}
		out[snake] = value
	}
	return out
}

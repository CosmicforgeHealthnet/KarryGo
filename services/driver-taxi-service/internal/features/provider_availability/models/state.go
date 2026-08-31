// Package availabilitymodels holds the provider operational state machine
// (addendum §5). It replaces the boolean online/offline that phase A4 built:
// a driver can be offered a ride, reserved for a scheduled pickup, en route,
// at pickup, on a trip, paused, or held for a safety/compliance reason, and
// only one of those ten states is actually eligible for a NEW offer.
package availabilitymodels

import "time"

// OperationalState is one of the ten states in the addendum §5 table.
type OperationalState string

const (
	StateOffline        OperationalState = "OFFLINE"
	StateAvailable      OperationalState = "AVAILABLE"
	StateOffered        OperationalState = "OFFERED"
	StateReserved       OperationalState = "RESERVED"
	StateEnRoutePickup  OperationalState = "EN_ROUTE_PICKUP"
	StateAtPickup       OperationalState = "AT_PICKUP"
	StateOnTrip         OperationalState = "ON_TRIP"
	StatePaused         OperationalState = "PAUSED"
	StateSafetyHold     OperationalState = "SAFETY_HOLD"
	StateComplianceHold OperationalState = "COMPLIANCE_HOLD"
)

// validStates backs Valid() with an O(1) lookup rather than a growing switch.
var validStates = map[OperationalState]bool{
	StateOffline:        true,
	StateAvailable:      true,
	StateOffered:        true,
	StateReserved:       true,
	StateEnRoutePickup:  true,
	StateAtPickup:       true,
	StateOnTrip:         true,
	StatePaused:         true,
	StateSafetyHold:     true,
	StateComplianceHold: true,
}

// Valid reports whether s is one of the ten defined states.
func (s OperationalState) Valid() bool { return validStates[s] }

// Eligible reports whether a provider in this state may receive a new ride
// offer. Only AVAILABLE is eligible — addendum §5 table, "Eligible for new
// ride?" column. Every other state, including OFFERED and RESERVED, is
// deliberately not eligible: a provider already mid-offer or mid-reservation
// must not be double-offered a second ride.
func (s OperationalState) Eligible() bool { return s == StateAvailable }

// Discoverable reports whether a provider in this state should still appear
// in proximity queries even though they cannot take a NEW offer — the
// three-state distinction the availability store already implements via
// SetDiscoverable (indexed-and-discoverable / indexed-but-hidden / gone).
// A provider ON_TRIP or PAUSED must remain trackable by their own passenger's
// app without being handed a second ride.
func (s OperationalState) Discoverable() bool {
	switch s {
	case StateOnTrip, StatePaused, StateEnRoutePickup, StateAtPickup:
		return true
	default:
		return false
	}
}

// State is the current-row shape of provider_operational_state.
type State struct {
	ProviderID  string
	Current     OperationalState
	Reason      string
	BookingID   *string
	EffectiveAt time.Time
	Version     int64
}

// HistoryEntry is one row of provider_operational_state_history — an
// append-only audit trail of every transition, per addendum architecture
// principle "safety decisions must be auditable".
type HistoryEntry struct {
	ID         string
	ProviderID string
	FromState  OperationalState
	ToState    OperationalState
	Reason     string
	BookingID  *string
	ChangedAt  time.Time
}

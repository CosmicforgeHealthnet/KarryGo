package logistics

// Normalized execution status (VND-020).
//
// Each authority keeps its own richer vocabulary — Dispatch has nineteen
// lifecycle states, Taxi Goods fifteen, Hauling eleven — and none of them are
// thrown away: the raw value is still stored on the event. What Vendor needs
// on top of that is one state it can reason about for every authority, because
// commerce logic ("has custody moved yet?", "is this finished?") must not have
// to branch on which driver service happens to be executing.
//
// The mapping is deliberately lossy in one direction only. A rich status maps
// to exactly one normalized state; a normalized state never maps back to a
// rich one.

import "strings"

// ExecutionStatus is the commerce-facing fulfilment state of one mission.
type ExecutionStatus string

const (
	// StatusUnknown means no mapping exists for the value supplied. It is
	// never stored as a fulfilment state; callers treat it as "do not
	// advance", because guessing is how a projection silently skips custody.
	StatusUnknown ExecutionStatus = ""

	StatusPending          ExecutionStatus = "PENDING"
	StatusAssigning        ExecutionStatus = "ASSIGNING"
	StatusAssigned         ExecutionStatus = "ASSIGNED"
	StatusProviderEnRoute  ExecutionStatus = "PROVIDER_EN_ROUTE"
	StatusArrivedPickup    ExecutionStatus = "ARRIVED_PICKUP"
	StatusPickupVerified   ExecutionStatus = "PICKUP_VERIFIED"
	StatusInTransit        ExecutionStatus = "IN_TRANSIT"
	StatusArrivedDelivery  ExecutionStatus = "ARRIVED_DELIVERY"
	StatusDeliveryVerified ExecutionStatus = "DELIVERY_VERIFIED"
	StatusCompleted        ExecutionStatus = "COMPLETED"
	StatusCancelled        ExecutionStatus = "CANCELLED"
	StatusFailed           ExecutionStatus = "FAILED"

	// StatusReconciliationRequired means Vendor cannot tell from its own
	// records what the authority believes, and an operator or a
	// reconciliation pass has to ask. It is not a failure: the mission may
	// well be running.
	StatusReconciliationRequired ExecutionStatus = "RECONCILIATION_REQUIRED"
)

// Terminal reports whether a status ends the mission's execution.
func (s ExecutionStatus) Terminal() bool {
	switch s {
	case StatusCompleted, StatusCancelled, StatusFailed:
		return true
	default:
		return false
	}
}

// CustodyWithProvider reports whether the provider is holding the goods.
//
// It is derived from the normalized state rather than from an authority's own
// custody table, because the one question commerce asks — can the vendor still
// stop this? — has the same answer whoever is executing.
func (s ExecutionStatus) CustodyWithProvider() bool {
	switch s {
	case StatusPickupVerified, StatusInTransit, StatusArrivedDelivery:
		return true
	default:
		return false
	}
}

// rank orders the progressive states so a late-arriving stale event cannot
// walk a mission backwards.
//
// Terminal states share the top rank: which terminal a mission reached is a
// fact the projection keeps, but no terminal may be overwritten by another.
func (s ExecutionStatus) rank() int {
	switch s {
	case StatusPending:
		return 1
	case StatusAssigning:
		return 2
	case StatusAssigned:
		return 3
	case StatusProviderEnRoute:
		return 4
	case StatusArrivedPickup:
		return 5
	case StatusPickupVerified:
		return 6
	case StatusInTransit:
		return 7
	case StatusArrivedDelivery:
		return 8
	case StatusDeliveryVerified:
		return 9
	case StatusCompleted, StatusCancelled, StatusFailed:
		return 10
	default:
		// Unknown and RECONCILIATION_REQUIRED carry no position: neither
		// says the mission moved.
		return 0
	}
}

// Advances reports whether moving from current to next is forward progress.
//
// A repeated event advances nothing, which is exactly what makes duplicate
// pickup and delivery callbacks safe to apply twice (VND-018, VND-019).
func Advances(current, next ExecutionStatus) bool {
	if next == StatusUnknown || next == StatusReconciliationRequired {
		return false
	}
	if current.Terminal() {
		return false
	}
	return next.rank() > current.rank()
}

// dispatchStatuses maps driver-dispatch-delivery-service's lifecycle states.
var dispatchStatuses = map[string]ExecutionStatus{
	"PLANNED":               StatusPending,
	"OFFERING":              StatusAssigning,
	"QUEUED":                StatusAssigning,
	"REASSIGNING":           StatusAssigning,
	"ASSIGNED":              StatusAssigned,
	"EN_ROUTE_PICKUP":       StatusProviderEnRoute,
	"AT_PICKUP":             StatusArrivedPickup,
	"PICKUP_VERIFICATION":   StatusArrivedPickup,
	"PICKED_UP":             StatusPickupVerified,
	"IN_TRANSIT":            StatusInTransit,
	"HUB_HOLD":              StatusInTransit,
	"RETURNING":             StatusInTransit,
	"AT_DELIVERY":           StatusArrivedDelivery,
	"DELIVERY_VERIFICATION": StatusArrivedDelivery,
	"DELIVERED":             StatusDeliveryVerified,
	"COMPLETED":             StatusCompleted,
	"CANCELLED":             StatusCancelled,
	"FAILED":                StatusFailed,
	"RETURNED":              StatusFailed,
}

// taxiStatuses maps driver-taxi-service's goods mission states.
var taxiStatuses = map[string]ExecutionStatus{
	"MATCHING":                   StatusAssigning,
	"OFFERED":                    StatusAssigning,
	"ASSIGNED":                   StatusAssigned,
	"EN_ROUTE_PICKUP":            StatusProviderEnRoute,
	"AT_PICKUP":                  StatusArrivedPickup,
	"PICKUP_HANDOVER_REQUIRED":   StatusArrivedPickup,
	"PICKED_UP":                  StatusPickupVerified,
	"IN_TRANSIT":                 StatusInTransit,
	"AT_DROPOFF":                 StatusArrivedDelivery,
	"DELIVERY_HANDOVER_REQUIRED": StatusArrivedDelivery,
	"DELIVERED":                  StatusDeliveryVerified,
	"COMPLETED":                  StatusCompleted,
	"CANCELLED":                  StatusCancelled,
	"UNMATCHED":                  StatusFailed,
	"FAILED":                     StatusFailed,
}

// haulingStatuses maps driver-hauling-service's booking states. Hauling stores
// them lower-case; comparison is case-folded before lookup.
var haulingStatuses = map[string]ExecutionStatus{
	"PENDING_MATCH":       StatusAssigning,
	"AWAITING_ACCEPTANCE": StatusAssigning,
	"ACCEPTED":            StatusAssigned,
	"EN_ROUTE_PICKUP":     StatusProviderEnRoute,
	"ARRIVED_AT_PICKUP":   StatusArrivedPickup,
	"PICKED_UP":           StatusPickupVerified,
	"EN_ROUTE_DELIVERY":   StatusInTransit,
	"DELIVERED":           StatusDeliveryVerified,
	"COMPLETED":           StatusCompleted,
	"CANCELLED":           StatusCancelled,
	"UNMATCHED":           StatusFailed,
}

// coarseStatuses are the four collapsed event names every authority already
// sends on the legacy vendor logistics-events boundary. They remain first-class
// so today's traffic keeps projecting while richer states are adopted.
var coarseStatuses = map[string]ExecutionStatus{
	"PICKED_UP":       StatusPickupVerified,
	"DELIVERING":      StatusInTransit,
	"DELIVERED":       StatusDeliveryVerified,
	"DELIVERY_FAILED": StatusFailed,

	// Mission-level answers from the commerce mission contract.
	"ACCEPTED":  StatusAssigning,
	"REFUSED":   StatusFailed,
	"CANCELLED": StatusCancelled,
}

// NormalizeStatus maps one authority's own status or event name to the
// commerce-facing state.
//
// The authority's table is consulted first so a value that means different
// things to different services resolves correctly, and the shared coarse
// vocabulary second so the legacy boundary keeps working. An unrecognised
// value returns StatusUnknown rather than a guess.
func NormalizeStatus(authority Authority, raw string) ExecutionStatus {
	value := TrimUpper(raw)
	if value == "" {
		return StatusUnknown
	}
	var table map[string]ExecutionStatus
	switch authority {
	case Dispatch:
		table = dispatchStatuses
	case Taxi:
		table = taxiStatuses
	case Hauling:
		table = haulingStatuses
	}
	if table != nil {
		if status, ok := table[value]; ok {
			return status
		}
	}
	if status, ok := coarseStatuses[value]; ok {
		return status
	}
	// A value that is already normalized passes through, so a projection can
	// be re-applied from its own stored state.
	if known := ExecutionStatus(value); known.rank() > 0 || known == StatusReconciliationRequired {
		return known
	}
	return StatusUnknown
}

// NormalizeStatusFor is NormalizeStatus for a caller holding the authority as
// a string, which is how it arrives on an HTTP boundary.
func NormalizeStatusFor(authority, raw string) ExecutionStatus {
	parsed, err := Parse(authority)
	if err != nil {
		// Without a known authority only the shared vocabulary is safe to
		// apply: another service's private spelling could mean anything.
		if status, ok := coarseStatuses[TrimUpper(raw)]; ok {
			return status
		}
		return StatusUnknown
	}
	return NormalizeStatus(parsed, raw)
}

// KnownStatusValues lists every canonical state, for contract tests and
// documentation. The order is the lifecycle order.
func KnownStatusValues() []ExecutionStatus {
	return []ExecutionStatus{
		StatusPending, StatusAssigning, StatusAssigned, StatusProviderEnRoute,
		StatusArrivedPickup, StatusPickupVerified, StatusInTransit,
		StatusArrivedDelivery, StatusDeliveryVerified, StatusCompleted,
		StatusCancelled, StatusFailed, StatusReconciliationRequired,
	}
}

// String makes ExecutionStatus printable without a conversion at each site.
func (s ExecutionStatus) String() string { return string(s) }

// EqualStatus compares two raw status spellings from the same authority.
func EqualStatus(authority Authority, left, right string) bool {
	normalized := NormalizeStatus(authority, left)
	if normalized == StatusUnknown {
		return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
	}
	return normalized == NormalizeStatus(authority, right)
}

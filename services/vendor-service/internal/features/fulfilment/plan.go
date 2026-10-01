package fulfilment

// The candidate fulfilment plan.
//
// Types only. The planning decision lives in planner.go and the persistence in
// store.go, so a plan can be reasoned about without either.

import (
	"encoding/json"
	"errors"
	"time"
)

// Authority is who executes a delivery.
type Authority string

const (
	// AuthorityNone is an unserviceable group. It is a real value rather than
	// an empty string so a group with no authority is never mistaken for one
	// whose authority was simply not filled in.
	AuthorityNone Authority = "NONE"

	AuthorityPlatform  Authority = "PLATFORM_LOGISTICS"
	AuthorityVendorOwn Authority = "VENDOR_OWN_DELIVERY"
)

// FeeBeneficiary is who the delivery fee is owed to.
//
// Derived from the authority, never configured. Vendor-owned delivery pays the
// vendor; platform delivery pays the executing provider and never the vendor.
type FeeBeneficiary string

const (
	BeneficiaryVendor           FeeBeneficiary = "VENDOR"
	BeneficiaryPlatformProvider FeeBeneficiary = "PLATFORM_PROVIDER"
)

// beneficiaryFor is the single place the rule is applied.
func beneficiaryFor(authority Authority) FeeBeneficiary {
	if authority == AuthorityVendorOwn {
		return BeneficiaryVendor
	}

	return BeneficiaryPlatformProvider
}

// PlanStatus is the plan lifecycle.
type PlanStatus string

const (
	StatusPlanning PlanStatus = "PLANNING"
	StatusPlanned  PlanStatus = "PLANNED"

	// StatusUnserviceable means at least one group cannot be delivered by any
	// authority. Checkout cannot proceed for that group, and the plan says
	// which and why rather than quietly omitting it.
	StatusUnserviceable PlanStatus = "UNSERVICEABLE"

	StatusExpired    PlanStatus = "EXPIRED"
	StatusSuperseded PlanStatus = "SUPERSEDED"
	StatusFailed     PlanStatus = "FAILED"
)

// Live reports whether the plan may still be acted on.
func (s PlanStatus) Live() bool {
	return s == StatusPlanning || s == StatusPlanned
}

// Mode is the optimizer mode a plan was produced under.
//
// SINGLE is the honest answer while one authoritative plan is produced. The
// three named modes exist for when the optimizer genuinely selects differently
// — three identical options with different labels would be a lie told three
// times, which the section explicitly forbids.
type Mode string

const (
	ModeSingle   Mode = "SINGLE"
	ModeFastest  Mode = "FASTEST"
	ModeBalanced Mode = "BALANCED"
	ModeSaver    Mode = "SAVER"
)

// Plan-level failure codes.
const (
	FailureGroupUnserviceable = "GROUP_UNSERVICEABLE"
	FailureNoAuthority        = "NO_LOGISTICS_AUTHORITY_CONFIGURED"
	FailureCurrencyMismatch   = "CURRENCY_MISMATCH"
	FailureAuthorityFault     = "LOGISTICS_AUTHORITY_UNAVAILABLE"
	FailureCheckoutNotUsable  = "CHECKOUT_NOT_PLANNABLE"
)

// Group-level reason codes owned by planning itself. Codes that arrive from a
// logistics authority or from the vendor-own evaluation are passed through
// unchanged, so a reason always names the party that produced it.
const (
	ReasonServiceable = "SERVICEABLE"

	// ReasonBranchInactive and ReasonBranchNotAcceptingOrders are pickup-side
	// refusals: the goods are sellable but nobody can collect them.
	ReasonBranchUnknown            = "PICKUP_BRANCH_UNKNOWN"
	ReasonBranchInactive           = "PICKUP_BRANCH_INACTIVE"
	ReasonBranchNotAcceptingOrders = "PICKUP_BRANCH_NOT_ACCEPTING_ORDERS"

	ReasonNoDeliveryAddress = "DELIVERY_ADDRESS_REQUIRED"

	// ReasonNoAuthorityAvailable means neither vendor-own delivery nor any
	// enabled platform authority could serve the group. It is the terminal
	// "checkout cannot proceed for this group" answer.
	ReasonNoAuthorityAvailable = "NO_DELIVERY_OPTION_AVAILABLE"

	ReasonAuthorityUnavailable = "LOGISTICS_AUTHORITY_UNAVAILABLE"
)

// Planning event actions.
const (
	EventPlanningStarted    = "commerce.fulfilment.planning_started"
	EventRequirementSolved  = "commerce.fulfilment.group_requirement_resolved"
	EventVendorOwnSelected  = "commerce.fulfilment.vendor_own_selected"
	EventQuoteRequested     = "commerce.fulfilment.quote_requested"
	EventQuoteReceived      = "commerce.fulfilment.quote_received"
	EventQuoteRefused       = "commerce.fulfilment.quote_refused"
	EventQuoteUnavailable   = "commerce.fulfilment.quote_unavailable"
	EventGroupUnserviceable = "commerce.fulfilment.group_unserviceable"
	EventConsolidated       = "commerce.fulfilment.consolidated"

	// EventCandidateSelected records which alternative the optimizer chose.
	// The alternatives themselves are persisted as rows, not as events; this
	// marks the moment in the plan history when the choice was made.
	EventCandidateSelected = "commerce.fulfilment.candidate_selected"
	EventPlanned           = "commerce.fulfilment.planned"
	EventFailed            = "commerce.fulfilment.failed"
	EventExpired           = "commerce.fulfilment.expired"
	EventSuperseded        = "commerce.fulfilment.superseded"
)

var (
	ErrPlanNotFound = errors.New("commerce fulfilment plan not found")

	ErrCheckoutNotPlannable = errors.New(
		"commerce checkout cannot be planned",
	)

	ErrPlanConflict = errors.New(
		"commerce fulfilment plan conflicts with the current checkout state",
	)

	ErrDependencyMissing = errors.New(
		"required fulfilment planning dependency is not connected",
	)
)

// Funding attributes each part of a delivery charge.
//
// The invariant — customer + vendor + platform equals gross — is what keeps
// "who actually paid for this delivery" answerable at settlement instead of
// being re-derived from business rules afterwards.
type Funding struct {
	GrossMinor          int64 `json:"gross_minor"`
	CustomerMinor       int64 `json:"customer_minor"`
	VendorFundedMinor   int64 `json:"vendor_funded_minor"`
	PlatformFundedMinor int64 `json:"platform_funded_minor"`
}

// Balanced states the funding invariant.
func (f Funding) Balanced() bool {
	return f.CustomerMinor+f.VendorFundedMinor+f.PlatformFundedMinor ==
		f.GrossMinor
}

// Add accumulates one funding split into another.
func (f Funding) Add(other Funding) Funding {
	return Funding{
		GrossMinor:          f.GrossMinor + other.GrossMinor,
		CustomerMinor:       f.CustomerMinor + other.CustomerMinor,
		VendorFundedMinor:   f.VendorFundedMinor + other.VendorFundedMinor,
		PlatformFundedMinor: f.PlatformFundedMinor + other.PlatformFundedMinor,
	}
}

// QuoteEvidence is the logistics authority's own answer, recorded verbatim.
//
// Vendor Service invents none of these fields. An empty QuoteID on a platform
// mission is refused by the database, which is what makes "no fake fee"
// structural rather than a convention.
type QuoteEvidence struct {
	QuoteID        string `json:"quote_id,omitempty"`
	QuoteReference string `json:"quote_reference,omitempty"`
	Revision       int64  `json:"revision,omitempty"`

	PickupRef  string `json:"pickup_ref,omitempty"`
	DropoffRef string `json:"dropoff_ref,omitempty"`

	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

// Breakdown is the display decomposition of a fee.
type Breakdown struct {
	BaseMinor      int64 `json:"base_minor"`
	DistanceMinor  int64 `json:"distance_minor"`
	SurchargeMinor int64 `json:"surcharge_minor"`
}

// Mission is one quoted execution unit.
//
// It is the COMMERCIAL unit: one authoritative quote, one fee. A later section
// may split it into several physical execution missions without destroying
// that commitment, so nothing here assumes one mission is one trip.
type Mission struct {
	ID string `json:"id"`

	Authority Authority `json:"authority"`
	Service   Service   `json:"service"`

	VehicleClass VehicleClass `json:"vehicle_class,omitempty"`

	PickupBusinessID string  `json:"pickup_business_id"`
	PickupBranchID   string  `json:"pickup_branch_id"`
	PickupLatitude   float64 `json:"-"`
	PickupLongitude  float64 `json:"-"`

	DropoffLatitude  float64 `json:"-"`
	DropoffLongitude float64 `json:"-"`

	Consolidated bool `json:"consolidated"`

	Quote QuoteEvidence `json:"quote,omitempty"`

	DistanceKm       float64 `json:"distance_km"`
	EstimatedMinutes int     `json:"estimated_minutes"`

	Currency string  `json:"currency,omitempty"`
	Funding  Funding `json:"funding"`

	Breakdown Breakdown `json:"breakdown"`

	FeeBeneficiary              FeeBeneficiary `json:"fee_beneficiary"`
	DeliveryMode                string         `json:"delivery_mode,omitempty"`
	FallbackTrigger             string         `json:"fallback_trigger,omitempty"`
	PolicyVersion               int64          `json:"policy_version,omitempty"`
	CustomerPriceGuaranteeMinor int64          `json:"customer_price_guarantee_minor"`
	CustodyState                string         `json:"custody_state"`

	TotalWeightGrams       int64 `json:"total_weight_grams"`
	PackageCount           int64 `json:"package_count"`
	LoadingDurationSeconds int64 `json:"loading_duration_seconds,omitempty"`

	// GroupIDs is what makes a consolidated fee traceable: every mission
	// records exactly which vendor groups it covers.
	GroupIDs []string `json:"group_ids"`
}

// PlanGroup is one vendor/branch group in the plan, with its own outcome.
type PlanGroup struct {
	ID string `json:"id"`

	CheckoutGroupID string `json:"checkout_group_id"`

	BusinessID string `json:"business_id"`
	BranchID   string `json:"branch_id"`

	Serviceable bool   `json:"serviceable"`
	ReasonCode  string `json:"reason_code"`

	Authority Authority `json:"authority"`

	MissionID string `json:"mission_id,omitempty"`

	VehicleClass VehicleClass `json:"vehicle_class,omitempty"`

	Requirement Requirement `json:"requirement"`

	// RequirementSnapshot is the immutable evidence behind this group's
	// vehicle decision: the measurements it was taken from, the capability
	// policy in force, every class considered and why the chosen one was
	// chosen. It is what makes "why a van" answerable months later, after the
	// thresholds have moved and the integrations have changed.
	//
	// It is nil only for a group refused before any vehicle was considered -
	// an unknown branch, an invalid address - where no vehicle question was
	// ever asked.
	RequirementSnapshot *RequirementSnapshot `json:"requirement_snapshot,omitempty"`

	Funding                     Funding `json:"funding"`
	DeliveryMode                string  `json:"delivery_mode,omitempty"`
	FallbackTrigger             string  `json:"fallback_trigger,omitempty"`
	CustomerPriceGuaranteeMinor int64   `json:"customer_price_guarantee_minor"`
}

// Plan is the candidate fulfilment plan for one checkout session.
type Plan struct {
	ID string `json:"id"`

	CustomerID string `json:"-"`

	CheckoutSessionID          string `json:"checkout_session_id"`
	CheckoutValidationRevision int64  `json:"checkout_validation_revision"`

	Status PlanStatus `json:"status"`

	// Mode is the fulfilment preference this plan was optimized for. It flows
	// unchanged into the CheckoutQuote as selected_fulfilment_mode, so a plan
	// quoted as SAVER can never be silently executed as FASTEST against a
	// charge the customer did not approve.
	Mode Mode `json:"mode"`

	// OptimizationPolicyVersion is the threshold envelope the candidate
	// comparison was decided under. Thresholds move; an explanation that
	// silently re-reads today numbers has stopped explaining what happened.
	OptimizationPolicyVersion int64 `json:"optimization_policy_version"`

	PlanRevision int64 `json:"plan_revision"`

	Currency                            string          `json:"currency,omitempty"`
	Funding                             Funding         `json:"delivery_funding"`
	DeliveryPolicySnapshot              json.RawMessage `json:"delivery_policy_snapshot,omitempty"`
	CustomerDeliveryPriceGuaranteeMinor int64           `json:"customer_delivery_price_guarantee_minor"`
	DeliveryPriceGuaranteeCurrency      string          `json:"delivery_price_guarantee_currency,omitempty"`

	FailureCode string `json:"failure_code,omitempty"`

	ExpiresAt time.Time `json:"expires_at"`

	Version int64 `json:"version"`

	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	PlannedAt *time.Time `json:"planned_at,omitempty"`

	Missions []Mission   `json:"missions"`
	Groups   []PlanGroup `json:"groups"`

	// Candidates are every alternative the optimizer considered, selected and
	// rejected alike, each with the reason for its fate.
	//
	// Keeping the losers is the whole point. "Why were these vendors
	// consolidated", "why did we split the order" and "why could SAVER not
	// consolidate these pickups" are all questions about an alternative that
	// did not run, and none of them is answerable from a plan that stored
	// only the one that did.
	Candidates []PlanCandidate `json:"candidates,omitempty"`
}

// SelectedCandidate returns the alternative that was executed.
func (p Plan) SelectedCandidate() (PlanCandidate, bool) {
	for _, candidate := range p.Candidates {
		if candidate.Status == StatusCandidateSelected {
			return candidate, true
		}
	}

	return PlanCandidate{}, false
}

// Expired reports whether the plan has lapsed at the given instant.
func (p Plan) Expired(now time.Time) bool {
	return !p.ExpiresAt.After(now)
}

// Unserviceable lists the groups that cannot be delivered.
//
// A caller must be able to tell the customer exactly which vendors failed and
// why, which is why the plan never simply omits them.
func (p Plan) Unserviceable() []PlanGroup {
	var refused []PlanGroup

	for _, group := range p.Groups {
		if !group.Serviceable {
			refused = append(refused, group)
		}
	}

	return refused
}

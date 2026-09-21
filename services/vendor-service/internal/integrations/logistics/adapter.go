package logistics

// Authority adapters, capability, serviceability and readiness
// (VND-005 … VND-009, VND-013, VND-030, VND-031).
//
// The rule this file exists to enforce: Vendor asks the authority what it can
// do instead of deciding for it. "Small parcels are Dispatch, big loads are
// Hauling" is a frontend heuristic, and heuristics drift away from the service
// they describe silently — Vendor would keep sending three-pickup missions to
// a service that has quietly become single-pickup, and only discover it when a
// customer's order failed in the field.
//
// So capability is a query with an answer, a version and an observation time,
// and every constraint Vendor enforces is one the authority declared.
//
// Nothing here prices anything. Capability says what can be carried; what it
// costs is the quote contract's business and is not touched.

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/logisticscapability"
)

// Capability and Requirement are the shared contract's own types. Vendor does
// not define a parallel shape for them: a second shape is a second thing to
// keep in step.
type (
	Capability     = logisticscapability.Capability
	Requirement    = logisticscapability.Requirement
	Serviceability = logisticscapability.Serviceability
)

// CapabilityAdapter is the read side of an authority adapter (VND-013).
//
// It is separate from mission creation because a deployment may legitimately
// be able to ask an authority what it can do while being unable to send it
// work, and conflating the two would make a capability query fail for the
// wrong reason.
type CapabilityAdapter interface {
	CheckCapability(ctx context.Context) (Capability, error)
	CheckServiceability(ctx context.Context, requirement Requirement) (Serviceability, error)
}

// Readiness is one authority's usable state (VND-031).
type Readiness struct {
	Authority Authority `json:"authority"`

	// Enabled is the deployment's feature flag for this authority. A disabled
	// authority is never selected, and never silently replaced by another
	// (VND-032).
	Enabled bool `json:"enabled"`

	// Configured means credentials and a base URL are present. Enabled
	// without configured is a deployment error, not a runtime fallback.
	Configured bool `json:"configured"`

	// Reachable is the result of the last capability query. It is
	// deliberately not cached forever: an authority that answered an hour ago
	// is not evidence it is answering now.
	Reachable  bool      `json:"reachable"`
	CheckedAt  time.Time `json:"checked_at,omitempty"`
	ReasonCode string    `json:"reason_code,omitempty"`
}

// Usable reports whether Vendor may select this authority for new work.
func (r Readiness) Usable() bool { return r.Enabled && r.Configured }

// Reason codes for readiness and selection outcomes. They are codes rather
// than sentences so an operator surface and a test can both match on them.
const (
	ReasonReady            = "READY"
	ReasonDisabled         = "AUTHORITY_DISABLED"
	ReasonNotConfigured    = "AUTHORITY_NOT_CONFIGURED"
	ReasonUnreachable      = "AUTHORITY_UNREACHABLE"
	ReasonNotPlatform      = "NOT_A_PLATFORM_AUTHORITY"
	ReasonUnknownAuthority = "UNKNOWN_AUTHORITY"
)

// Registry is Vendor's view of the logistics authorities it may use.
//
// It holds one capability adapter per authority and the deployment's flags. It
// is safe for concurrent use: after construction it is only read.
type Registry struct {
	adapters map[Authority]CapabilityAdapter
	enabled  map[Authority]bool
	now      func() time.Time
}

// RegistryOptions configures a Registry.
type RegistryOptions struct {
	// Adapters is the capability adapter per authority. An authority with no
	// adapter is treated as not configured rather than assumed capable.
	Adapters map[Authority]CapabilityAdapter

	// Enabled is the deployment's per-authority feature flag.
	Enabled map[Authority]bool

	Now func() time.Time
}

// NewRegistry builds a registry. A nil map is an empty map, so a deployment
// that wires nothing gets an honest "nothing is available" rather than a panic.
func NewRegistry(options RegistryOptions) *Registry {
	registry := &Registry{
		adapters: map[Authority]CapabilityAdapter{},
		enabled:  map[Authority]bool{},
		now:      options.Now,
	}
	if registry.now == nil {
		registry.now = time.Now
	}
	for authority, adapter := range options.Adapters {
		if adapter == nil {
			continue
		}
		registry.adapters[authority] = adapter
	}
	for authority, on := range options.Enabled {
		registry.enabled[authority] = on
	}
	return registry
}

// NewRegistryFromClientSet builds a registry over the shared capability client
// set, which is how a real deployment wires it.
func NewRegistryFromClientSet(clients logisticscapability.Set, enabled map[Authority]bool, now func() time.Time) *Registry {
	adapters := map[Authority]CapabilityAdapter{}
	for _, authority := range PlatformAuthorities() {
		if client, ok := clients.For(authority); ok {
			adapters[authority] = clientAdapter{client: client}
		}
	}
	return NewRegistry(RegistryOptions{Adapters: adapters, Enabled: enabled, Now: now})
}

// clientAdapter binds the shared capability client to the adapter interface.
type clientAdapter struct{ client *logisticscapability.Client }

func (a clientAdapter) CheckCapability(ctx context.Context) (Capability, error) {
	return a.client.Capability(ctx)
}

func (a clientAdapter) CheckServiceability(ctx context.Context, requirement Requirement) (Serviceability, error) {
	return a.client.Serviceability(ctx, requirement)
}

// Enabled reports the deployment flag for an authority.
func (r *Registry) Enabled(authority Authority) bool {
	if r == nil {
		return false
	}
	return r.enabled[authority]
}

// Configured reports whether an adapter exists for an authority.
func (r *Registry) Configured(authority Authority) bool {
	if r == nil {
		return false
	}
	_, ok := r.adapters[authority]
	return ok
}

// Usable reports whether an authority may be selected at all.
//
// It is checked before any network call, so a disabled authority costs
// nothing and — more importantly — an authority that is switched off cannot be
// reached by accident through a capability probe.
func (r *Registry) Usable(authority Authority) bool {
	return r.Enabled(authority) && r.Configured(authority)
}

// Readiness probes one authority (VND-031).
//
// A disabled or unconfigured authority is reported without being contacted.
// A configured one is contacted, and a failure is reported as unreachable
// rather than raised: readiness is a status surface, not an operation.
func (r *Registry) Readiness(ctx context.Context, authority Authority) Readiness {
	state := Readiness{Authority: authority, Enabled: r.Enabled(authority), Configured: r.Configured(authority)}
	if !authority.PlatformExecuted() {
		state.ReasonCode = ReasonNotPlatform
		return state
	}
	if !state.Enabled {
		state.ReasonCode = ReasonDisabled
		return state
	}
	if !state.Configured {
		state.ReasonCode = ReasonNotConfigured
		return state
	}
	state.CheckedAt = r.now().UTC()
	capability, err := r.adapters[authority].CheckCapability(ctx)
	if err != nil {
		state.ReasonCode = ReasonUnreachable
		return state
	}
	state.Reachable = capability.Available
	if capability.Available {
		state.ReasonCode = ReasonReady
	} else {
		state.ReasonCode = capability.UnavailableReason
		if state.ReasonCode == "" {
			state.ReasonCode = ReasonUnreachable
		}
	}
	return state
}

// ReadinessAll probes every platform authority, in a stable order.
func (r *Registry) ReadinessAll(ctx context.Context) []Readiness {
	states := make([]Readiness, 0, 3)
	for _, authority := range PlatformAuthorities() {
		states = append(states, r.Readiness(ctx, authority))
	}
	return states
}

// ErrAuthorityNotUsable means the authority is disabled or unconfigured in
// this deployment.
var ErrAuthorityNotUsable = errors.New("logistics: authority is not usable in this deployment")

// Capability returns one authority's declared capability.
func (r *Registry) Capability(ctx context.Context, authority Authority) (Capability, error) {
	if !r.Usable(authority) {
		return Capability{}, ErrAuthorityNotUsable
	}
	return r.adapters[authority].CheckCapability(ctx)
}

// Serviceability asks one authority whether it can execute a requirement
// (VND-009).
//
// The answer is the authority's, not Vendor's. Vendor only refuses before
// asking, and only for the two things it knows without asking: an authority it
// may not use, and an authority that is not a platform executor at all.
func (r *Registry) Serviceability(ctx context.Context, authority Authority, requirement Requirement) (Serviceability, error) {
	if !authority.PlatformExecuted() {
		return Serviceability{Authority: authority, Serviceable: false, ReasonCode: ReasonNotPlatform}, nil
	}
	if !r.Usable(authority) {
		reason := ReasonNotConfigured
		if !r.Enabled(authority) {
			reason = ReasonDisabled
		}
		return Serviceability{Authority: authority, Serviceable: false, ReasonCode: reason}, nil
	}
	return r.adapters[authority].CheckServiceability(ctx, requirement)
}

// Eligible returns every authority that can execute a requirement, in the
// platform order (VND-030).
//
// It does not rank, choose or split. Which of several eligible authorities a
// commerce plan uses is a planning decision made elsewhere with the customer's
// approved terms in hand; this answers only which of them are permitted to be
// considered at all. Automatic splitting or rerouting is deliberately absent:
// the spec makes that an explicit product rule, not a fallback.
func (r *Registry) Eligible(ctx context.Context, requirement Requirement) []Serviceability {
	results := make([]Serviceability, 0, 3)
	for _, authority := range PlatformAuthorities() {
		answer, err := r.Serviceability(ctx, authority, requirement)
		if err != nil {
			failure := Classify(authority, err)
			results = append(results, Serviceability{
				Authority: authority, Serviceable: false,
				ReasonCode: string(failure.Category),
			})
			continue
		}
		results = append(results, answer)
	}
	return results
}

// EligibleAuthorities is Eligible reduced to the authorities that answered yes.
func (r *Registry) EligibleAuthorities(ctx context.Context, requirement Requirement) []Authority {
	eligible := make([]Authority, 0, 3)
	for _, answer := range r.Eligible(ctx, requirement) {
		if answer.Serviceable {
			eligible = append(eligible, answer.Authority)
		}
	}
	sort.Slice(eligible, func(i, j int) bool { return eligible[i] < eligible[j] })
	return eligible
}

// Authorize is the gate a mission must pass before it is sent (VND-005 … 008,
// VND-030).
//
// It returns a classified failure rather than a bare error so the caller has a
// customer-safe category and the authority's own reason code, and so a refusal
// is distinguishable from an outage: the first must not be retried, the second
// must.
func (r *Registry) Authorize(ctx context.Context, authority Authority, requirement Requirement) error {
	if !authority.PlatformExecuted() {
		return NewFailure(authority, FailureCapabilityMismatch, ReasonNotPlatform)
	}
	if !r.Enabled(authority) {
		return NewFailure(authority, FailureDependencyUnavailable, ReasonDisabled)
	}
	if !r.Configured(authority) {
		return NewFailure(authority, FailureDependencyUnavailable, ReasonNotConfigured)
	}
	answer, err := r.adapters[authority].CheckServiceability(ctx, requirement)
	if err != nil {
		return Classify(authority, err)
	}
	if answer.Serviceable {
		return nil
	}
	return NewFailure(authority, categoryForReason(answer.ReasonCode), answer.ReasonCode)
}

// CategoryForReasonCode maps a capability refusal code onto the Vendor-safe
// category.
//
// It is exported because the orchestration planner classifies the SAME codes
// when it evaluates candidates, and a second copy of this mapping is a second
// thing to keep in step -- the drift would show up as an authority that is
// permanently eliminated for what was really an outage.
func CategoryForReasonCode(reason string) FailureCategory { return categoryForReason(reason) }

// categoryForReason maps the capability contract's refusal codes onto the
// Vendor-safe categories.
//
// Every physical-limit refusal is a capability mismatch; an authority that is
// simply switched off or unreachable is a dependency problem. Anything the
// contract may add later that is not recognised here becomes NOT_SERVICEABLE,
// which is the honest answer for an unexplained refusal: it will not be
// retried, and it will not be reported as an outage.
func categoryForReason(reason string) FailureCategory {
	switch strings.ToUpper(strings.TrimSpace(reason)) {
	case "", logisticscapability.ReasonAuthorityUnavailable, ReasonDisabled, ReasonNotConfigured, ReasonUnreachable:
		return FailureDependencyUnavailable
	case logisticscapability.ReasonSecurePickupMissing, logisticscapability.ReasonSecureDeliveryMissing,
		logisticscapability.ReasonSecurePickupLimit:
		// A protected handover this authority cannot issue is a credential
		// problem, not a capacity one: the goods would fit, but the pickup
		// could not be verified, and a protected handover never downgrades to
		// an unverified one.
		return FailureCredential
	case logisticscapability.ReasonMultiPickupNotAllowed, logisticscapability.ReasonTooManyPickups,
		logisticscapability.ReasonMultiStopNotAllowed, logisticscapability.ReasonTooManyDropoffs,
		logisticscapability.ReasonWeightExceeded, logisticscapability.ReasonVolumeExceeded,
		logisticscapability.ReasonDimensionExceeded, logisticscapability.ReasonPackageCountExceeded,
		logisticscapability.ReasonVehicleUnsupported, logisticscapability.ReasonHandlingUnsupported:
		return FailureCapabilityMismatch
	default:
		return FailureNotServiceable
	}
}

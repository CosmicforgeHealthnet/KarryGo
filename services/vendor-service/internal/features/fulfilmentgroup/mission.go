package fulfilmentgroup

// The two mission types, kept deliberately apart.
//
// PlatformMissionRef is a PROJECTION of a mission owned by Dispatch, Hauling or
// Taxi. VendorMission is CANONICAL, and only exists because the vendor itself
// is the executor. They are different types because they answer to different
// authorities, and one ambiguous mission entity would make "whose word is
// final" unanswerable exactly when a delivery goes wrong.

import (
	"encoding/json"
	"strings"
	"time"
)

// LinkState is where a platform mission reference stands.
//
// It describes THIS SERVICE'S relationship to the canonical mission — whether
// we have asked for it, bound it, or replaced it — and never the mission's own
// operational progress, which is what StatusProjection carries.
type LinkState string

const (
	// LinkRequested means creation has been asked for but no canonical
	// reference is bound yet. It is the state a lost response leaves behind,
	// and the state recovery reads back rather than blindly re-creating from.
	LinkRequested LinkState = "REQUESTED"

	LinkLinked LinkState = "LINKED"

	// LinkRefused is the authority's own answer that it will not execute. A
	// real answer, not a fault: retrying it forever would strand the order.
	LinkRefused LinkState = "REFUSED"

	LinkFailed LinkState = "FAILED"

	// LinkSuperseded means a newer plan version moved the work elsewhere.
	LinkSuperseded LinkState = "SUPERSEDED"

	// LinkReplaced means the canonical mission itself was replaced. The row
	// stays readable so the operational history survives.
	LinkReplaced LinkState = "REPLACED"
)

// Valid reports whether the link state is known.
func (l LinkState) Valid() bool {
	switch l {
	case LinkRequested, LinkLinked, LinkRefused, LinkFailed,
		LinkSuperseded, LinkReplaced:
		return true
	default:
		return false
	}
}

// Occupies reports whether this reference still holds its plan mission slot.
func (l LinkState) Occupies() bool {
	return l == LinkRequested || l == LinkLinked
}

// PlatformMissionRef is Vendor Service's projection of somebody else's mission.
//
// Everything that could tempt this service into acting as the authority is
// absent by construction: there is no provider identity, no route state, no
// ability to set a status from a staff action, and ExternalRef is treated as
// an opaque string that is never parsed for meaning.
type PlatformMissionRef struct {
	ID string `json:"platform_mission_ref_id"`

	GroupID       string `json:"fulfilment_group_id"`
	PlanID        string `json:"plan_id"`
	PlanMissionID string `json:"plan_mission_id"`

	// Authority is stated explicitly rather than inferred from the shape of
	// ExternalRef. Reading business meaning out of another service's
	// identifier is how a projection quietly becomes a parser.
	Authority Authority `json:"authority"`

	// MissionRef is OUR operation identity and the authority's idempotency
	// key. The same MissionRef must always return the same canonical mission.
	MissionRef string `json:"mission_ref"`

	// ExternalRef is the authority's own opaque booking or trip identity.
	ExternalRef string `json:"external_ref,omitempty"`

	LinkState LinkState `json:"link_state"`

	// StatusProjection is a cached copy for vendor coordination. It always
	// names the source version that produced it, so an older event that
	// arrives late is recognised as older rather than applied as newer.
	StatusProjection         string          `json:"status_projection,omitempty"`
	DispatchDomainProjection json.RawMessage `json:"dispatch_domain_projection,omitempty"`
	StatusSourceVersion      int64           `json:"status_source_version"`
	StatusSourceRef          string          `json:"status_source_ref,omitempty"`
	StatusUpdatedAt          *time.Time      `json:"status_updated_at,omitempty"`

	RefusalCode   string `json:"refusal_code,omitempty"`
	FailureReason string `json:"failure_reason,omitempty"`

	// ReplacesID keeps the chain when a canonical mission is replaced. The
	// old row is not overwritten: the mission it names really happened.
	ReplacesID        string `json:"replaces_id,omitempty"`
	ReplacementReason string `json:"replacement_reason,omitempty"`

	Version   int64      `json:"version"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	LinkedAt  *time.Time `json:"linked_at,omitempty"`
}

// Live reports whether this reference is the mission currently executing the
// slot.
func (r PlatformMissionRef) Live() bool { return r.LinkState.Occupies() }

// AcceptStatusProjection reports whether an incoming status may be applied.
//
// Equal versions are refused as well as older ones: a redelivered event
// carries the same version, and applying it again would rewrite the projection
// timestamp with no new information. This is the guard that stops a delayed
// IN_TRANSIT from un-delivering a delivered mission.
func (r PlatformMissionRef) AcceptStatusProjection(sourceVersion int64) bool {
	if sourceVersion < 1 {
		return false
	}
	return sourceVersion > r.StatusSourceVersion
}

// MissionLink is the authority's answer being bound to a slot.
type MissionLink struct {
	ExternalRef string
	Accepted    bool
	RefusalCode string
}

// Validate reports the first reason an answer could not be bound.
func (l MissionLink) Validate() error {
	if l.Accepted {
		if strings.TrimSpace(l.ExternalRef) == "" {
			// An accepted mission with no canonical reference is a mission
			// nobody can follow.
			return ErrInvalid
		}
		return nil
	}
	if strings.TrimSpace(l.RefusalCode) == "" {
		// A refusal with no reason cannot be acted on, only retried blindly.
		return ErrInvalid
	}
	return nil
}

// VendorMissionStatus is the vendor-owned delivery lifecycle.
//
// These values are the delivery feature's existing ExecutionState, reused
// verbatim. Vendor-owned execution already had one lifecycle in this service
// and a second spelling of it would split the same journey across two
// vocabularies.
type VendorMissionStatus string

const (
	VendorMissionPlanned            VendorMissionStatus = "PLANNED"
	VendorMissionAssigned           VendorMissionStatus = "ASSIGNED"
	VendorMissionReadyForPickup     VendorMissionStatus = "READY_FOR_PICKUP"
	VendorMissionCustodyTransferred VendorMissionStatus = "CUSTODY_TRANSFERRED"
	VendorMissionOutForDelivery     VendorMissionStatus = "OUT_FOR_DELIVERY"
	VendorMissionDelivered          VendorMissionStatus = "DELIVERED"
	VendorMissionFailed             VendorMissionStatus = "FAILED"
	VendorMissionReturnRequired     VendorMissionStatus = "RETURN_REQUIRED"
	VendorMissionReturned           VendorMissionStatus = "RETURNED"
)

// Valid reports whether the status is known.
func (s VendorMissionStatus) Valid() bool {
	switch s {
	case VendorMissionPlanned, VendorMissionAssigned, VendorMissionReadyForPickup,
		VendorMissionCustodyTransferred, VendorMissionOutForDelivery,
		VendorMissionDelivered, VendorMissionFailed,
		VendorMissionReturnRequired, VendorMissionReturned:
		return true
	default:
		return false
	}
}

// InCustody reports whether the vendor rider is holding the goods.
func (s VendorMissionStatus) InCustody() bool {
	switch s {
	case VendorMissionCustodyTransferred, VendorMissionOutForDelivery,
		VendorMissionReturnRequired:
		return true
	default:
		return false
	}
}

// VendorMission is a delivery this service genuinely owns.
//
// It exists only when the vendor is the executor. A platform-executed delivery
// never has a row here — it has a PlatformMissionRef instead — so the presence
// of a VendorMission is itself the statement that the vendor is responsible.
type VendorMission struct {
	ID string `json:"vendor_mission_id"`

	GroupID       string `json:"fulfilment_group_id"`
	PlanID        string `json:"plan_id"`
	PlanMissionID string `json:"plan_mission_id"`

	BusinessID string `json:"business_id"`
	BranchID   string `json:"branch_id"`

	// AssignedStaffID is a vendor staff member holding the vendor-rider role.
	// This service keeps no second rider identity, and a vendor rider is never
	// a KarryGo marketplace provider.
	AssignedStaffID string     `json:"assigned_staff_id,omitempty"`
	AssignedAt      *time.Time `json:"assigned_at,omitempty"`

	Status VendorMissionStatus `json:"status"`

	// HandoverRef and ProofRef point at credentials and evidence issued by
	// their owning authority. The reference is recorded; the secret is not.
	HandoverRef string `json:"handover_ref,omitempty"`
	ProofRef    string `json:"proof_ref,omitempty"`

	FailureReason string `json:"failure_reason,omitempty"`

	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Stops     []Stop   `json:"stops,omitempty"`
	ParcelIDs []string `json:"parcel_ids,omitempty"`
}

// VendorMissionTransition is a requested lifecycle change with its evidence.
type VendorMissionTransition struct {
	To VendorMissionStatus

	ExpectedVersion int64

	HandoverRef   string
	ProofRef      string
	FailureReason string
}

// vendorMissionTransitions is the legal successor set for each status.
//
// It mirrors delivery.Transition exactly. It is restated rather than imported
// because that function speaks in the delivery feature's own type; keeping the
// table here and pinning the agreement in a test is safer than a conversion
// that would silently drift if either side changed.
var vendorMissionTransitions = map[VendorMissionStatus][]VendorMissionStatus{
	VendorMissionPlanned: {
		VendorMissionAssigned, VendorMissionFailed,
	},
	VendorMissionAssigned: {
		VendorMissionReadyForPickup, VendorMissionFailed,
	},
	VendorMissionReadyForPickup: {
		VendorMissionCustodyTransferred, VendorMissionFailed,
		VendorMissionReturnRequired,
	},
	VendorMissionCustodyTransferred: {
		VendorMissionOutForDelivery, VendorMissionReturnRequired,
	},
	VendorMissionOutForDelivery: {
		VendorMissionDelivered, VendorMissionFailed,
		VendorMissionReturnRequired,
	},
	VendorMissionReturnRequired: {
		VendorMissionReturned,
	},
}

// TransitionVendorMission reports whether a lifecycle change is legal.
//
// An arbitrary jump straight to DELIVERED is refused. That refusal is the
// point: a status field that any caller can set to "delivered" is not delivery
// evidence, it is a text box.
func TransitionVendorMission(from, to VendorMissionStatus) error {
	if !from.Valid() || !to.Valid() {
		return ErrInvalid
	}
	if from == to {
		return nil
	}
	for _, allowed := range vendorMissionTransitions[from] {
		if allowed == to {
			return nil
		}
	}
	return ErrConflict
}

// Validate reports the first reason a transition could not be applied.
func (t VendorMissionTransition) Validate(current VendorMission) error {
	if !t.To.Valid() {
		return ErrInvalid
	}
	if err := TransitionVendorMission(current.Status, t.To); err != nil {
		return err
	}
	// A mission cannot move past assignment with nobody assigned to perform
	// it: an unassigned mission that reports itself delivered has no rider to
	// have delivered it.
	if t.To != VendorMissionPlanned && strings.TrimSpace(current.AssignedStaffID) == "" {
		return ErrConflict
	}
	if t.To == VendorMissionFailed && strings.TrimSpace(t.FailureReason) == "" {
		return ErrInvalid
	}
	return nil
}

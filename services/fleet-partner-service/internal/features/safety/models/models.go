// Package models holds the fleet-side safety reference domain model.
//
// Fleet stores POINTERS to incidents and disputes; it never stores the case
// itself. Everything here is therefore an opaque identifier plus the fleet-side
// associations that make it findable, and nothing here is authoritative about
// what the case says.
package models

import "time"

// Reference types.
const (
	// ReferenceIncident points at a complaint or safety incident owned by the
	// support-dispute service.
	ReferenceIncident = "INCIDENT"
	// ReferenceDispute points at a dispute owned by the support-dispute
	// service.
	ReferenceDispute = "DISPUTE"
)

// AuthoritySupportDispute is the only authority fleet references today.
//
// Stored on every row rather than assumed, so a second authority can be added
// later without every existing row silently changing meaning.
const AuthoritySupportDispute = "SUPPORT_DISPUTE"

// Verification states.
const (
	// VerificationVerified means the authoritative service confirmed the
	// reference exists. It does NOT mean the organization owns the case, and
	// it does not confer authority to act.
	VerificationVerified = "VERIFIED"
	// VerificationUnverified means no authoritative endpoint exists to confirm
	// this reference. A record in this state is a note for humans and must
	// never authorize an operational or financial consequence.
	VerificationUnverified = "UNVERIFIED"
)

// Actor types.
const (
	ActorOrganizationUser = "ORGANIZATION_USER"
	ActorProvider         = "PROVIDER"
	ActorSystem           = "SYSTEM"
)

// History actions.
const (
	ActionCreated         = "CREATED"
	ActionStatusProjected = "STATUS_PROJECTED"
	ActionVerified        = "VERIFIED"
	ActionDetached        = "DETACHED"
)

// ValidReferenceType reports whether a type is one this service stores.
func ValidReferenceType(referenceType string) bool {
	return referenceType == ReferenceIncident || referenceType == ReferenceDispute
}

// Reference is one fleet-side pointer to an authoritative case.
type Reference struct {
	ID                  string
	OrganizationID      string
	ReferenceType       string
	AuthorityService    string
	ExternalReference   string
	AssignmentID        *string
	RequestProjectionID *string
	TripProjectionID    *string
	PlatformTripID      *string
	ExecutingProviderID *string
	VehicleReference    *string
	VerificationState   string
	VerifiedAt          *time.Time
	StatusProjection    *string
	StatusProjectionAt  *time.Time
	CreatedByActorType  string
	CreatedByActorID    string
	CorrelationID       string
	CausationID         string
	IdempotencyKey      string
	RequestFingerprint  string
	Version             int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Verified reports whether the authoritative service confirmed this reference.
func (r Reference) Verified() bool {
	return r.VerificationState == VerificationVerified
}

// AuthorizesConsequences reports whether this reference may be used to justify
// an operational or financial action.
//
// The answer is: only if the authoritative service confirmed it. An unverified
// reference is a human-readable note. Treating it as grounds for withholding
// pay, standing down a provider or reversing a payment would let anyone with
// write access manufacture consequences by typing an identifier.
func (r Reference) AuthorizesConsequences() bool {
	return r.Verified()
}

// HasAssociation reports whether the reference points at anything on the fleet
// side. A reference attached to nothing cannot be acted on and is refused.
func (r Reference) HasAssociation() bool {
	return notEmpty(r.AssignmentID) || notEmpty(r.RequestProjectionID) ||
		notEmpty(r.TripProjectionID) || notEmpty(r.PlatformTripID) ||
		notEmpty(r.ExecutingProviderID)
}

func notEmpty(value *string) bool {
	return value != nil && *value != ""
}

// HistoryEntry is one append-only record of what happened to a reference.
type HistoryEntry struct {
	ID             string
	OrganizationID string
	ReferenceID    string
	Action         string
	FromState      string
	ToState        string
	ActorType      string
	ActorID        string
	Metadata       map[string]any
	CorrelationID  string
	CausationID    string
	OccurredAt     time.Time
}

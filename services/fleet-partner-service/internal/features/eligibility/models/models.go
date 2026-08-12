// Package models holds the eligibility decision domain.
//
// An eligibility answer is a decision, not a boolean. It records what was
// asked, which policy version decided it, which authoritative sources were
// consulted and at which versions, when it was taken and when it stops being
// usable. That shape is what lets a gate be audited afterwards and what stops
// a stale "yes" from authorizing work months later.
//
// The five statuses are not three plus two error cases. UNKNOWN and STALE are
// real answers meaning "nobody can currently vouch for this", and every gate
// treats them exactly as it treats INELIGIBLE: it blocks.
package models

import (
	"strings"
	"time"
)

// Subject types an eligibility decision can be about.
const (
	SubjectCompany  = "COMPANY"
	SubjectProvider = "PROVIDER"
	SubjectVehicle  = "VEHICLE"
)

// Decision statuses.
const (
	// StatusEligible is the only status that authorizes anything, and only
	// while the decision is current and unexpired.
	StatusEligible = "ELIGIBLE"
	// StatusIneligible is a definitive no from data that was available.
	StatusIneligible = "INELIGIBLE"
	// StatusUnknown means an authoritative source could not be consulted.
	StatusUnknown = "UNKNOWN"
	// StatusStale means the data consulted was older than policy allows.
	StatusStale = "STALE"
	// StatusReconciling means a disagreement between sources is being
	// resolved. Nothing may proceed on a value nobody currently vouches for.
	StatusReconciling = "RECONCILING"
)

// Operation types. These are policy keys as well as decision operations, so
// one name identifies both the gate and the rules governing it.
const (
	OperationCompanyActivation = "COMPANY_ACTIVATION"
	OperationProviderLinking   = "PROVIDER_LINKING"
	OperationVehicleLinking    = "VEHICLE_LINKING"
	OperationAssignment        = "ASSIGNMENT"
	OperationExecution         = "EXECUTION"
	OperationPaidFeature       = "PAID_FEATURE"
	OperationFinancialRelease  = "FINANCIAL_RELEASE"
	OperationFinancialReversal = "FINANCIAL_REVERSAL"
	OperationDocumentAction    = "DOCUMENT_ACTION"
)

// Actor types.
const (
	ActorOrganizationUser = "ORGANIZATION_USER"
	ActorProvider         = "PROVIDER"
	ActorPlatformService  = "PLATFORM_SERVICE"
	ActorSystem           = "SYSTEM"
)

// NormalizeActorType maps a caller's own actor vocabulary onto the four values
// a decision may record.
//
// Callers name actors in their own terms — "SERVICE", "service",
// "organization_user", "dispatcher". Rather than let an unrecognised value
// reach a CHECK constraint and turn a blocked operation into a 500, anything
// unfamiliar becomes SYSTEM: attributing an action to the platform is the
// conservative claim, and it is visibly less specific than a real actor.
func NormalizeActorType(actorType string) string {
	switch strings.ToUpper(strings.TrimSpace(actorType)) {
	case ActorOrganizationUser, "ORGANIZATION-USER", "USER", "STAFF":
		return ActorOrganizationUser
	case ActorProvider, "DRIVER", "RIDER":
		return ActorProvider
	case ActorPlatformService, "SERVICE", "PLATFORM-SERVICE":
		return ActorPlatformService
	default:
		return ActorSystem
	}
}

// Source freshness states. The vocabulary matches the finance projections so
// one reader does not have to learn two spellings of the same idea.
const (
	FreshnessCurrent     = "CURRENT"
	FreshnessStale       = "STALE"
	FreshnessGapDetected = "GAP_DETECTED"
	FreshnessUnknown     = "UNKNOWN"
	FreshnessReconciling = "RECONCILING"
)

// History actions.
const (
	ActionEvaluated   = "EVALUATED"
	ActionSuperseded  = "SUPERSEDED"
	ActionExpired     = "EXPIRED"
	ActionInvalidated = "INVALIDATED"
	ActionConsumed    = "CONSUMED"
)

// Controlled reason codes. Every value here exists as a row in
// fleet_partner_eligibility_reason_codes, and the foreign key means a typo
// fails the write rather than producing an unfilterable reason nobody can
// query for later.
const (
	ReasonCompanyNotFound             = "company_not_found"
	ReasonCompanyNotActive            = "company_not_active"
	ReasonCompanySuspended            = "company_suspended"
	ReasonCompanyArchived             = "company_archived"
	ReasonCompanyVerificationPending  = "company_verification_pending"
	ReasonCompanyVerificationRejected = "company_verification_rejected"
	ReasonCompanyVerificationUnknown  = "company_verification_unknown"
	ReasonCompanyNotApproved          = "company_not_approved"
	ReasonCompanyRestricted           = "company_restricted"
	ReasonCompanyCapabilityMissing    = "company_capability_missing"
	ReasonCompanyBranchInvalid        = "company_branch_invalid"
	ReasonCompanyDocumentMissing      = "company_document_missing"
	ReasonCompanyDocumentExpired      = "company_document_expired"
	ReasonCompanyDocumentRejected     = "company_document_rejected"
	ReasonCompanySubscriptionInactive = "company_subscription_inactive"
	ReasonCompanySubscriptionStale    = "company_subscription_stale"

	ReasonProviderNotFound            = "provider_not_found"
	ReasonProviderMembershipNotActive = "provider_membership_not_active"
	ReasonProviderSuspended           = "provider_suspended"
	ReasonProviderNotLinked           = "provider_not_linked"
	ReasonProviderUnverified          = "provider_unverified"
	ReasonProviderServiceMismatch     = "provider_service_mismatch"
	ReasonProviderBranchMismatch      = "provider_branch_mismatch"
	ReasonProviderConsentMissing      = "provider_consent_missing"
	ReasonProviderNotAvailable        = "provider_not_available"
	ReasonProviderShiftBlocked        = "provider_shift_blocked"
	ReasonProviderDocumentMissing     = "provider_document_missing"
	ReasonProviderDocumentExpired     = "provider_document_expired"
	ReasonProviderAssignmentConflict  = "provider_assignment_conflict"
	ReasonProviderProjectionStale     = "provider_projection_stale"

	ReasonVehicleNotFound           = "vehicle_not_found"
	ReasonVehicleOwnershipMismatch  = "vehicle_ownership_mismatch"
	ReasonVehicleProviderMismatch   = "vehicle_provider_mismatch"
	ReasonVehicleServiceMismatch    = "vehicle_service_mismatch"
	ReasonVehicleUnverified         = "vehicle_unverified"
	ReasonVehicleNotAvailable       = "vehicle_not_available"
	ReasonVehicleRestricted         = "vehicle_restricted"
	ReasonVehicleDocumentMissing    = "vehicle_document_missing"
	ReasonVehicleDocumentExpired    = "vehicle_document_expired"
	ReasonVehicleAssignmentConflict = "vehicle_assignment_conflict"

	// WP10 document reason codes. They describe a fleet-side document
	// reference and its media projection, and are deliberately distinct from
	// the company_document_* codes above: those describe the legacy onboarding
	// attachment, and a reader who cannot tell the two apart cannot tell which
	// record to go and fix.
	ReasonDocumentMissing         = "document_missing"
	ReasonDocumentNotVerified     = "document_not_verified"
	ReasonDocumentNotSafe         = "document_not_safe"
	ReasonDocumentExpired         = "document_expired"
	ReasonDocumentRevoked         = "document_revoked"
	ReasonDocumentReplaced        = "document_replaced"
	ReasonDocumentDeleted         = "document_deleted"
	ReasonDocumentProjectionStale = "document_projection_stale"
	ReasonDocumentReconciling     = "document_reconciling"

	ReasonAuthorityUnavailable   = "authority_unavailable"
	ReasonAuthorityNotConfigured = "authority_not_configured"
	ReasonAuthorityConflict      = "authority_conflict"
	ReasonSourceVersionStale     = "source_version_stale"
	ReasonDecisionExpired        = "decision_expired"
	ReasonPolicyNotFound         = "policy_not_found"
	ReasonSubjectOutOfScope      = "subject_out_of_scope"
)

// Check names a policy can require. A check that a policy does not name is not
// run; a check that it names and that cannot be answered blocks.
const (
	CheckLifecycle           = "lifecycle"
	CheckVerification        = "verification"
	CheckApproval            = "approval"
	CheckRestrictions        = "restrictions"
	CheckDocuments           = "documents"
	CheckBranch              = "branch"
	CheckCapability          = "capability"
	CheckSubscription        = "subscription"
	CheckProjectionFreshness = "projection_freshness"

	CheckExistence          = "existence"
	CheckMembership         = "membership"
	CheckServiceType        = "service_type"
	CheckConsent            = "consent"
	CheckAvailability       = "availability"
	CheckShift              = "shift"
	CheckAssignmentConflict = "assignment_conflict"

	CheckOwnership           = "ownership"
	CheckProviderAssociation = "provider_association"
	CheckSourceVersion       = "source_version"
)

// Policy is one immutable version of the rules governing an operation.
type Policy struct {
	ID string
	// OrganizationID is empty for a platform default that tenants inherit.
	OrganizationID string
	PolicyKey      string
	OperationType  string
	Version        int64

	RequiredCompanyChecks  []string
	RequiredProviderChecks []string
	RequiredVehicleChecks  []string
	RequiredDocuments      []string

	DecisionTTLSeconds  int
	SourceMaxAgeSeconds int

	ApprovalRequired    bool
	MinimumApprovals    int
	EligibleRoleIDs     []string
	SelfApprovalAllowed bool
	SeparationOfDuties  bool
	ApprovalTTLSeconds  int

	AcknowledgementRequired bool
	SubscriptionRequired    bool

	FinancialThresholdMinor    *int64
	FinancialThresholdCurrency string

	Status        string
	EffectiveFrom time.Time
	CreatedAt     time.Time
}

// RequiresCompanyCheck reports whether this policy demands a company check.
func (p Policy) RequiresCompanyCheck(name string) bool {
	return contains(p.RequiredCompanyChecks, name)
}

// RequiresProviderCheck reports whether this policy demands a provider check.
func (p Policy) RequiresProviderCheck(name string) bool {
	return contains(p.RequiredProviderChecks, name)
}

// RequiresVehicleCheck reports whether this policy demands a vehicle check.
func (p Policy) RequiresVehicleCheck(name string) bool {
	return contains(p.RequiredVehicleChecks, name)
}

// DecisionTTL is how long an answer under this policy stays usable.
func (p Policy) DecisionTTL() time.Duration {
	return time.Duration(p.DecisionTTLSeconds) * time.Second
}

// SourceMaxAge is how old authoritative data may be before STALE.
func (p Policy) SourceMaxAge() time.Duration {
	return time.Duration(p.SourceMaxAgeSeconds) * time.Second
}

// ApprovalTTL is how long an approval under this policy stays usable.
func (p Policy) ApprovalTTL() time.Duration {
	return time.Duration(p.ApprovalTTLSeconds) * time.Second
}

func contains(values []string, name string) bool {
	for _, value := range values {
		if value == name {
			return true
		}
	}
	return false
}

// Decision is one recorded eligibility answer.
type Decision struct {
	ID             string
	OrganizationID string
	BranchID       *string
	SubjectType    string
	SubjectID      string
	OperationType  string

	PolicyID      string
	PolicyKey     string
	PolicyVersion int64

	Status           string
	Reasons          []string
	Sources          []DecisionSource
	InputFingerprint string

	CheckedAt    time.Time
	ExpiresAt    time.Time
	SupersededAt *time.Time
	SupersededBy *string

	ActorType string
	ActorID   string

	CorrelationID string
	CausationID   string
	AuditRecordID string
	OutboxEventID string

	CreatedAt time.Time
}

// Current reports whether this decision has not been replaced.
func (d Decision) Current() bool { return d.SupersededAt == nil }

// Expired reports whether the decision has passed its expiry.
func (d Decision) Expired(now time.Time) bool { return !d.ExpiresAt.After(now) }

// Authorizes reports whether this decision may authorize a sensitive
// operation right now.
//
// Four conditions, all required. Dropping any one of them is how a gate stops
// being a gate: a superseded decision was replaced for a reason, an expired
// one describes a world that has moved on, and every status other than
// ELIGIBLE means nobody is vouching for the subject.
func (d Decision) Authorizes(now time.Time) bool {
	return d.Status == StatusEligible && d.Current() && !d.Expired(now)
}

// BlockingReason names why this decision does not authorize, for a caller that
// needs to tell the user something specific.
func (d Decision) BlockingReason(now time.Time) string {
	switch {
	case d.Status == StatusEligible && !d.Current():
		return ReasonDecisionExpired
	case d.Status == StatusEligible && d.Expired(now):
		return ReasonDecisionExpired
	case len(d.Reasons) > 0:
		return d.Reasons[0]
	default:
		return ReasonAuthorityUnavailable
	}
}

// DecisionSource is one authoritative source consulted for a decision.
type DecisionSource struct {
	ID               string
	DecisionID       string
	OrganizationID   string
	AuthorityService string
	SourceKind       string
	SourceReference  string
	SourceVersion    int64
	FreshnessState   string
	ObservedAt       time.Time
}

// HistoryEntry is one append-only record of what happened to a decision.
type HistoryEntry struct {
	ID             string
	OrganizationID string
	DecisionID     string
	SubjectType    string
	SubjectID      string
	OperationType  string
	Action         string
	FromStatus     string
	ToStatus       string
	ActorType      string
	ActorID        string
	ReasonCode     string
	CorrelationID  string
	CausationID    string
	OccurredAt     time.Time
}

// ValidSubjectType reports whether a subject type is one this service decides.
func ValidSubjectType(subjectType string) bool {
	return subjectType == SubjectCompany || subjectType == SubjectProvider ||
		subjectType == SubjectVehicle
}

// ValidStatus reports whether a status is one of the five.
func ValidStatus(status string) bool {
	switch status {
	case StatusEligible, StatusIneligible, StatusUnknown, StatusStale, StatusReconciling:
		return true
	default:
		return false
	}
}

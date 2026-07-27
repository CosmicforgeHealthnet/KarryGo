// Package models holds the marketplace, assignment and trip-projection
// domain models.
//
// Every reference to an execution-service record is an opaque identifier
// carried with source metadata. The execution service owns request and trip
// truth; this package models the organization's own decisions plus read
// projections of that truth.
package models

import "time"

// Source services.
const (
	SourceDispatch = "DISPATCH"
	SourceTaxi     = "TAXI"
	SourceHauling  = "HAULING"
)

// Organization operating modes.
const (
	ModeCentralized = "CENTRALIZED"
	ModeDelegated   = "DELEGATED"
	ModeHybrid      = "HYBRID"
)

// AllowsDispatcherAcceptance reports whether a dispatcher may accept for the
// organization in this mode.
func AllowsDispatcherAcceptance(mode string) bool {
	return mode == ModeCentralized || mode == ModeHybrid
}

// AllowsProviderDirectAcceptance reports whether a provider may accept a
// marketplace offer directly in this mode.
func AllowsProviderDirectAcceptance(mode string) bool {
	return mode == ModeDelegated || mode == ModeHybrid
}

// Acceptance sources.
const (
	AcceptanceDispatcher       = "ORGANIZATION_DISPATCHER"
	AcceptanceProviderDirect   = "PROVIDER_DIRECT"
	AcceptancePlatformRecovery = "PLATFORM_RECOVERY"
	// AcceptanceAutomationDraft may only produce a recommendation or draft
	// assignment. It can never accept work autonomously.
	AcceptanceAutomationDraft = "AUTOMATION_DRAFT"
)

// Marketplace request projection statuses.
const (
	ProjectionOffered                = "OFFERED"
	ProjectionReservationPending     = "RESERVATION_PENDING"
	ProjectionOrganizationAccepted   = "ORGANIZATION_ACCEPTED"
	ProjectionAssignmentPending      = "ASSIGNMENT_PENDING"
	ProjectionProviderAccepted       = "PROVIDER_ACCEPTED"
	ProjectionExecutionRequested     = "EXECUTION_REQUESTED"
	ProjectionExecutionCreated       = "EXECUTION_CREATED"
	ProjectionInProgress             = "IN_PROGRESS"
	ProjectionCompleted              = "COMPLETED"
	ProjectionOrganizationDeclined   = "ORGANIZATION_DECLINED"
	ProjectionExpired                = "EXPIRED"
	ProjectionCancelled              = "CANCELLED"
	ProjectionReconciliationRequired = "RECONCILIATION_REQUIRED"
)

// Freshness states.
const (
	FreshnessCurrent     = "CURRENT"
	FreshnessStale       = "STALE"
	FreshnessGapDetected = "GAP_DETECTED"
	FreshnessReconciling = "RECONCILING"
	FreshnessUnknown     = "UNKNOWN"
)

// BlocksStateChangingCommands reports whether a projection is too uncertain to
// act on. A material gap or unknown freshness must not be used to authorize a
// reservation or an assignment.
func BlocksStateChangingCommands(freshness string) bool {
	return freshness == FreshnessGapDetected || freshness == FreshnessUnknown
}

// RequestProjection is the organization-facing view of a marketplace request.
type RequestProjection struct {
	ID                       string
	OrganizationID           string
	SourceService            string
	PlatformRequestID        string
	PlatformBookingID        *string
	PlatformTripID           *string
	ServiceType              string
	ProjectionStatus         string
	SourceStatus             string
	PickupSummary            map[string]any
	DropoffSummary           map[string]any
	OperationalRequirements  map[string]any
	SettlementOrganizationID *string
	ExecutingProviderID      *string
	OrganizationMembershipID *string
	AcceptanceSource         *string
	AssignmentID             *string
	OfferedAt                time.Time
	ExpiresAt                *time.Time
	SourceVersion            int64
	LastEventID              *string
	LastSourceEventAt        *time.Time
	LastReconciledAt         *time.Time
	FreshnessState           string
	Version                  int64
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// Offer statuses.
const (
	OfferOffered   = "OFFERED"
	OfferReserved  = "RESERVED"
	OfferAccepted  = "ACCEPTED"
	OfferDeclined  = "DECLINED"
	OfferExpired   = "EXPIRED"
	OfferCancelled = "CANCELLED"
)

// Offer is the organization's opportunity on a marketplace request.
type Offer struct {
	ID                  string
	OrganizationID      string
	RequestProjectionID string
	OperatingMode       string
	Status              string
	OfferedAt           time.Time
	ExpiresAt           time.Time
	ReservedAt          *time.Time
	AcceptedAt          *time.Time
	DeclinedAt          *time.Time
	CancelledAt         *time.Time
	ResolvedActorType   *string
	ResolvedActorID     *string
	SourceReservationID *string
	ReasonCode          *string
	ReasonText          *string
	Version             int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// Actionable reports whether the offer can still be accepted or declined.
func (o Offer) Actionable(now time.Time) bool {
	return o.Status == OfferOffered && now.Before(o.ExpiresAt)
}

// Recipient statuses.
const (
	RecipientPending   = "PENDING"
	RecipientAccepted  = "ACCEPTED"
	RecipientDeclined  = "DECLINED"
	RecipientExpired   = "EXPIRED"
	RecipientCancelled = "CANCELLED"
	RecipientLostRace  = "LOST_RACE"
)

// OfferRecipient is one managed provider's copy of an organization offer.
type OfferRecipient struct {
	ID               string
	OrganizationID   string
	OfferID          string
	MembershipID     string
	Status           string
	DeliveryStatus   string
	DeliveredAt      *time.Time
	ViewedAt         *time.Time
	RespondedAt      *time.Time
	ClientResponseID *string
	ReasonCode       *string
	ReasonText       *string
	ExpiresAt        time.Time
	Version          int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// Actionable reports whether this provider can still accept or decline.
func (r OfferRecipient) Actionable(now time.Time) bool {
	return r.Status == RecipientPending && now.Before(r.ExpiresAt)
}

// Acceptance is the immutable record of the organization committing.
type Acceptance struct {
	ID                     string
	OrganizationID         string
	OfferID                string
	AcceptedByUserID       *string
	AcceptedByMembershipID *string
	AcceptanceSource       string
	SourceReservationID    string
	SourceIdempotencyKey   string
	Snapshot               map[string]any
	RequestID              string
	CorrelationID          string
	AcceptedAt             time.Time
}

// Assignment statuses.
const (
	AssignmentCreated                 = "CREATED"
	AssignmentPendingProviderResponse = "PENDING_PROVIDER_RESPONSE"
	AssignmentAccepted                = "ACCEPTED"
	AssignmentReadyForExecution       = "READY_FOR_EXECUTION"
	AssignmentInProgress              = "IN_PROGRESS"
	AssignmentCompleted               = "COMPLETED"
	AssignmentDeclined                = "DECLINED"
	AssignmentExpired                 = "EXPIRED"
	AssignmentCancelled               = "CANCELLED"
	AssignmentExecutionStartPending   = "EXECUTION_START_PENDING"
	AssignmentReconciliationRequired  = "RECONCILIATION_REQUIRED"
)

// Execution-start statuses.
const (
	ExecutionNotRequested           = "NOT_REQUESTED"
	ExecutionPending                = "PENDING"
	ExecutionSubmitted              = "SUBMITTED"
	ExecutionConfirmed              = "CONFIRMED"
	ExecutionRejected               = "REJECTED"
	ExecutionUnknownOutcome         = "UNKNOWN_OUTCOME"
	ExecutionReconciliationRequired = "RECONCILIATION_REQUIRED"
	ExecutionFailedPermanent        = "FAILED_PERMANENT"
)

// Assignment is a provider's offered piece of work.
type Assignment struct {
	ID                   string
	OrganizationID       string
	RequestProjectionID  string
	OfferID              *string
	MembershipID         string
	ExecutingProviderID  string
	SourceService        string
	AssetReference       *string
	AssignedByType       string
	AssignedByID         string
	Status               string
	ResponseDeadline     time.Time
	AcceptedAt           *time.Time
	DeclinedAt           *time.Time
	ExpiredAt            *time.Time
	CancelledAt          *time.Time
	ReadyForExecutionAt  *time.Time
	StartedAt            *time.Time
	CompletedAt          *time.Time
	RootAssignmentID     *string
	ReplacesAssignmentID *string
	ReassignmentAttempt  int
	ReassignmentReason   *string
	ExecutionStartStatus string
	SourceExecutionID    *string
	Version              int64
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// AllowedActions reports what the bound provider may do right now.
//
// This is the server-authoritative answer to the universal Accept/Decline
// rule: while an assignment awaits a response the provider always has BOTH
// actions, and no organization setting, role, mode or employment relationship
// can remove decline. Once resolved, no action remains.
func (a Assignment) AllowedActions(now time.Time) []string {
	if a.Status == AssignmentPendingProviderResponse && now.Before(a.ResponseDeadline) {
		return []string{"accept", "decline"}
	}
	return []string{}
}

// Resolved reports whether a provider decision or terminal outcome exists.
func (a Assignment) Resolved() bool {
	switch a.Status {
	case AssignmentAccepted, AssignmentReadyForExecution, AssignmentInProgress,
		AssignmentCompleted, AssignmentDeclined, AssignmentExpired, AssignmentCancelled,
		AssignmentExecutionStartPending:
		return true
	default:
		return false
	}
}

// assignmentTransitions is the binding assignment state machine.
var assignmentTransitions = map[string]map[string]bool{
	AssignmentCreated: {
		AssignmentPendingProviderResponse: true,
		AssignmentCancelled:               true,
	},
	AssignmentPendingProviderResponse: {
		AssignmentAccepted:  true,
		AssignmentDeclined:  true,
		AssignmentExpired:   true,
		AssignmentCancelled: true,
	},
	AssignmentAccepted: {
		AssignmentExecutionStartPending:  true,
		AssignmentReadyForExecution:      true,
		AssignmentReconciliationRequired: true,
		AssignmentCancelled:              true,
	},
	AssignmentExecutionStartPending: {
		AssignmentReadyForExecution:      true,
		AssignmentReconciliationRequired: true,
		AssignmentCancelled:              true,
	},
	AssignmentReadyForExecution: {
		AssignmentInProgress: true,
		AssignmentCancelled:  true,
	},
	AssignmentInProgress: {
		AssignmentCompleted: true,
		AssignmentCancelled: true,
	},
	AssignmentReconciliationRequired: {
		AssignmentReadyForExecution: true,
		AssignmentInProgress:        true,
		AssignmentCancelled:         true,
	},
	// Terminal.
	AssignmentCompleted: {},
	AssignmentDeclined:  {},
	AssignmentExpired:   {},
	AssignmentCancelled: {},
}

// CanTransition reports whether an assignment move is allowed.
func CanTransition(from string, to string) bool {
	next, ok := assignmentTransitions[from]
	if !ok {
		return false
	}
	return next[to]
}

// NeedsReassignment reports whether a resolved assignment leaves the request
// needing another provider decision.
func NeedsReassignment(status string) bool {
	return status == AssignmentDeclined || status == AssignmentExpired
}

// Response is the immutable provider decision on an assignment.
type Response struct {
	ID               string
	OrganizationID   string
	AssignmentID     string
	MembershipID     string
	Response         string
	ReasonCode       *string
	ReasonText       *string
	SafetyRelated    bool
	ClientResponseID string
	DeviceRecordedAt *time.Time
	OccurredAt       time.Time
	RequestID        string
	CorrelationID    string
	EvidenceFileIDs  []string
}

// Response values.
const (
	ResponseAccepted = "ACCEPTED"
	ResponseDeclined = "DECLINED"
)

// StateHistoryEntry is one append-only assignment transition.
type StateHistoryEntry struct {
	ID             string
	OrganizationID string
	AssignmentID   string
	FromStatus     string
	ToStatus       string
	ActorType      string
	ActorID        string
	ReasonCode     *string
	Metadata       map[string]any
	RequestID      string
	CorrelationID  string
	OccurredAt     time.Time
}

// Reservation resource types.
const (
	ResourceProvider = "PROVIDER"
	ResourceAsset    = "ASSET"
)

// Reservation holds provider or asset capacity for a live assignment.
type Reservation struct {
	ID             string
	OrganizationID string
	AssignmentID   string
	ResourceType   string
	ResourceID     string
	Status         string
	ReleasedAt     *time.Time
}

// Reassignment triggers.
const (
	TriggerDeclined        = "DECLINED"
	TriggerExpired         = "EXPIRED"
	TriggerCancelled       = "CANCELLED"
	TriggerEligibilityLost = "ELIGIBILITY_LOST"
)

// ReassignmentCase tracks a request needing another provider.
type ReassignmentCase struct {
	ID                    string
	OrganizationID        string
	RequestProjectionID   string
	OriginAssignmentID    string
	Trigger               string
	ReasonCode            *string
	RequiresHumanReview   bool
	Status                string
	Attempts              int
	ExcludedMembershipIDs []string
	ResolvedAssignmentID  *string
	Version               int64
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// TripProjection is the organization-facing view of a canonical trip.
type TripProjection struct {
	ID                       string
	OrganizationID           string
	SourceService            string
	PlatformTripID           string
	PlatformRequestID        *string
	PlatformBookingID        *string
	ExecutingProviderID      string
	OrganizationMembershipID *string
	SettlementOrganizationID *string
	AssignmentID             *string
	ServiceType              string
	TripStatus               string
	SourceVersion            int64
	LastEventID              *string
	LastEventAt              *time.Time
	LastReconciledAt         *time.Time
	FreshnessState           string
	CompletedAt              *time.Time
	CancelledAt              *time.Time
	Version                  int64
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// Command operation types and statuses.
const (
	CommandReserveRequest     = "RESERVE_REQUEST"
	CommandReleaseReservation = "RELEASE_RESERVATION"
	CommandConfirmProvider    = "CONFIRM_PROVIDER"
	CommandCreateExecution    = "CREATE_EXECUTION"
	CommandCancelExecution    = "CANCEL_EXECUTION"

	CommandPending                = "PENDING"
	CommandSubmitted              = "SUBMITTED"
	CommandConfirmed              = "CONFIRMED"
	CommandRejected               = "REJECTED"
	CommandUnknownOutcome         = "UNKNOWN_OUTCOME"
	CommandReconciliationRequired = "RECONCILIATION_REQUIRED"
	CommandFailedPermanent        = "FAILED_PERMANENT"
)

// CommandOperation records every external state-changing call so a timeout
// can be resolved by read-back instead of a blind retry.
type CommandOperation struct {
	ID                 string
	OrganizationID     string
	OperationType      string
	SourceService      string
	SourceRequestID    string
	AssignmentID       *string
	IdempotencyKey     string
	RequestFingerprint string
	Status             string
	ExternalReference  *string
	Attempts           int
	LastErrorCode      *string
	OutcomeCheckedAt   *time.Time
	NextAttemptAt      *time.Time
	Version            int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// NeedsReadBack reports whether the outcome of a command is uncertain and
// must be resolved by asking the source service.
func (c CommandOperation) NeedsReadBack() bool {
	return c.Status == CommandUnknownOutcome || c.Status == CommandReconciliationRequired ||
		c.Status == CommandSubmitted
}

// ExternalReferenceOrEmpty returns the stored external reference, or empty.
func (c CommandOperation) ExternalReferenceOrEmpty() string {
	if c.ExternalReference == nil {
		return ""
	}
	return *c.ExternalReference
}

// ExecutionStartOperation is durable work to create the canonical execution.
//
// It exists so a provider's acceptance and the obligation to act on it are
// committed together. Statuses mirror the external-outcome vocabulary used
// across this service: PENDING/CLAIMED are local, EXECUTION_START_PENDING
// means a source call is in flight, and UNKNOWN_OUTCOME means the answer must
// be resolved by read-back rather than guessed.
type ExecutionStartOperation struct {
	ID                       string
	OrganizationID           string
	AssignmentID             string
	ProviderResponseID       string
	RequestProjectionID      string
	SourceService            string
	IdempotencyKey           string
	RequestFingerprint       string
	Status                   string
	AttemptCount             int
	NextAttemptAt            time.Time
	LeaseOwner               *string
	LeaseExpiresAt           *time.Time
	DispatchResponseSnapshot []byte
	TripID                   *string
	LastError                *string
	Version                  int64
	CreatedAt                time.Time
	UpdatedAt                time.Time
	CompletedAt              *time.Time
}

// Execution-start operation statuses.
const (
	ExecutionStartPending         = "PENDING"
	ExecutionStartClaimed         = "CLAIMED"
	ExecutionStartInFlight        = "EXECUTION_START_PENDING"
	ExecutionStartConfirmed       = "CONFIRMED"
	ExecutionStartRejected        = "REJECTED"
	ExecutionStartUnknown         = "UNKNOWN_OUTCOME"
	ExecutionStartRetryScheduled  = "RETRY_SCHEDULED"
	ExecutionStartReconcileNeeded = "RECONCILIATION_REQUIRED"
	ExecutionStartDeadLetter      = "DEAD_LETTER"
)

// NeedsReadBack reports whether the operation must be resolved by asking the
// source service what happened, rather than by retrying the command.
func (o ExecutionStartOperation) NeedsReadBack() bool {
	return o.Status == ExecutionStartUnknown || o.Status == ExecutionStartInFlight
}

// Package models holds the approval domain.
//
// This extends the approval framework created in migration 000006 rather than
// replacing it. The tables, the append-only decision log and the one-open-
// request-per-resource rule are unchanged; what is added here is the meaning
// of an approval at execution time — what exactly was approved, against which
// version of the subject, and whether that is still true.
package models

import "time"

// Request statuses.
const (
	StatusPending   = "PENDING"
	StatusApproved  = "APPROVED"
	StatusRejected  = "REJECTED"
	StatusExpired   = "EXPIRED"
	StatusCancelled = "CANCELLED"
	// StatusInvalidated means the approval no longer describes what would be
	// executed. It is deliberately not EXPIRED (which says "wait and it will
	// be re-approvable") and not REJECTED (which says "a person said no").
	StatusInvalidated = "INVALIDATED"
	StatusExecuted    = "EXECUTED"
)

// Decision values.
const (
	DecisionApproved = "APPROVED"
	DecisionRejected = "REJECTED"
)

// Execution outcomes.
const (
	OutcomeExecuted = "EXECUTED"
	OutcomeRejected = "REJECTED"
	OutcomeUnknown  = "UNKNOWN"
)

// Invalidation reasons. A closed vocabulary so the reason can be filtered on
// and translated, and so it can never carry free text about a person.
const (
	InvalidationPayloadChanged        = "payload_changed"
	InvalidationSubjectVersionChanged = "subject_version_changed"
	InvalidationEligibilityLost       = "eligibility_lost"
	InvalidationPolicyChanged         = "policy_changed"
	InvalidationSupersededByRequest   = "superseded_by_request"
)

// Actor types.
const (
	ActorOrganizationUser = "ORGANIZATION_USER"
	ActorSystem           = "SYSTEM"
)

// Request is one approval request for a controlled operation.
type Request struct {
	ID             string
	OrganizationID string
	BranchID       *string
	PolicyID       string
	PolicyVersion  int64

	ResourceType string
	ResourceID   string
	Action       string
	RequesterID  string

	// RequestFingerprint is the hash of the normalized controlled action.
	// Execution must present the same value.
	RequestFingerprint string
	PayloadVersion     int
	PayloadSnapshot    map[string]any

	// ExpectedSubjectVersion is the subject version the approval was granted
	// against. Zero means the requester asserted none.
	ExpectedSubjectVersion int64

	MinimumApprovals    int
	EligibleRoleIDs     []string
	SelfApprovalAllowed bool
	SeparationOfDuties  bool

	EligibilityDecisionID *string

	Status             string
	CurrentStep        int
	ExpiresAt          time.Time
	ExecutedAt         *time.Time
	ExecutionReference *string
	InvalidatedAt      *time.Time
	InvalidationReason string

	IdempotencyKey string
	CorrelationID  string
	CausationID    string
	Version        int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Open reports whether the request can still receive decisions.
func (r Request) Open(now time.Time) bool {
	return r.Status == StatusPending && r.ExpiresAt.After(now)
}

// Executable reports whether this request may be spent on an operation.
//
// Four conditions, all required, and each corresponds to a way approvals get
// subverted in practice: executing a request nobody approved, executing one
// that was withdrawn, executing after the window closed, and executing twice.
func (r Request) Executable(now time.Time) bool {
	return r.Status == StatusApproved && r.ExpiresAt.After(now) &&
		r.ExecutedAt == nil && r.InvalidatedAt == nil
}

// Terminal reports whether the request can never change again.
func (r Request) Terminal() bool {
	switch r.Status {
	case StatusRejected, StatusExpired, StatusCancelled, StatusInvalidated, StatusExecuted:
		return true
	default:
		return false
	}
}

// Decision is one immutable approver decision.
type Decision struct {
	ID                  string
	OrganizationID      string
	ApprovalRequestID   string
	ApprovalStepID      string
	ApproverID          string
	Decision            string
	Reason              string
	ClientDecisionID    *string
	StepUpReference     *string
	RequestVersion      int64
	ApprovedFingerprint string
	ActorRoleID         string
	CorrelationID       string
	DecidedAt           time.Time
}

// Step is one approval stage of a request.
type Step struct {
	ID                string
	OrganizationID    string
	ApprovalRequestID string
	StepNumber        int
	RequiredQuorum    int
	EligibleRoleIDs   []string
	Status            string
	CompletedAt       *time.Time
	Version           int64
}

// Execution is the durable record of spending an approval.
type Execution struct {
	ID                     string
	OrganizationID         string
	ApprovalRequestID      string
	PresentedFingerprint   string
	ApprovedFingerprint    string
	ExpectedSubjectVersion int64
	ObservedSubjectVersion int64
	ExecutionReference     string
	Outcome                string
	SafeReasonCode         string
	ActorType              string
	ActorID                string
	IdempotencyKey         string
	CorrelationID          string
	CausationID            string
	OccurredAt             time.Time
}

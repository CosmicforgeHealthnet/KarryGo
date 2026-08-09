// Package models holds the fleet-side document domain.
//
// Fleet stores a POINTER to a file plus everything the business needs to know
// about it: whose it is, what category it is, when it expires, whether a human
// verified it, whether it was replaced or revoked. Fleet stores nothing about
// the file itself — not the bytes, not the location, not the detected type,
// not the safety verdict — except as a projection of what media-file-service
// said, carrying the version that answer belonged to.
//
// The distinction is what the whole package protects. Fleet may decide that a
// verified insurance certificate satisfies a requirement; it may never decide
// that the file is safe, that it still exists, or that it is really a PDF.
package models

import "time"

// Owner types a document can belong to.
const (
	OwnerCompany      = "COMPANY"
	OwnerBranch       = "BRANCH"
	OwnerProviderLink = "PROVIDER_LINK"
	OwnerVehicleLink  = "VEHICLE_LINK"
	OwnerIncident     = "INCIDENT"
	OwnerDispute      = "DISPUTE"
	OwnerApproval     = "APPROVAL"
	// OwnerOrder links a document to a private order. Orders carry a
	// CustomerID, which is what makes a customer-facing document surface
	// possible: authorization walks document -> order -> CustomerID rather
	// than requiring documents to know about customers directly.
	OwnerOrder = "ORDER"
)

// ValidOwnerType reports whether a type is one this service stores.
func ValidOwnerType(ownerType string) bool {
	switch ownerType {
	case OwnerCompany, OwnerBranch, OwnerProviderLink, OwnerVehicleLink,
		OwnerIncident, OwnerDispute, OwnerApproval, OwnerOrder:
		return true
	default:
		return false
	}
}

// Document categories.
const (
	CategoryInsurance          = "INSURANCE"
	CategoryRegistration       = "REGISTRATION"
	CategoryPermit             = "PERMIT"
	CategoryCompliance         = "COMPLIANCE"
	CategoryProof              = "PROOF"
	CategoryIncidentAttachment = "INCIDENT_ATTACHMENT"
	CategoryDisputeAttachment  = "DISPUTE_ATTACHMENT"
	CategoryApprovalAttachment = "APPROVAL_ATTACHMENT"
	CategorySupporting         = "SUPPORTING"
)

// ValidCategory reports whether a category is one this service stores.
func ValidCategory(category string) bool {
	switch category {
	case CategoryInsurance, CategoryRegistration, CategoryPermit, CategoryCompliance,
		CategoryProof, CategoryIncidentAttachment, CategoryDisputeAttachment,
		CategoryApprovalAttachment, CategorySupporting:
		return true
	default:
		return false
	}
}

// Visibility tiers. INTERNAL is the default: a document is corporate-only
// until something explicitly widens or restricts it.
const (
	VisibilityInternal            = "INTERNAL"
	VisibilityProviderVisible     = "PROVIDER_VISIBLE"
	VisibilityCustomerVisible     = "CUSTOMER_VISIBLE"
	VisibilityRestricted          = "RESTRICTED"
	VisibilityCompanyConfidential = "COMPANY_CONFIDENTIAL"
)

var validVisibilities = map[string]bool{
	VisibilityInternal: true, VisibilityProviderVisible: true, VisibilityCustomerVisible: true,
	VisibilityRestricted: true, VisibilityCompanyConfidential: true,
}

// ValidVisibility reports whether the tier is one this service recognizes.
func ValidVisibility(visibility string) bool { return validVisibilities[visibility] }

// RequiresElevatedAccess reports whether downloading a document at this
// visibility tier requires the separate documents.download_restricted
// permission on top of ordinary documents.download — a caller who can
// download ordinary documents does not automatically gain access to the
// most sensitive tier.
func RequiresElevatedAccess(visibility string) bool {
	return visibility == VisibilityRestricted || visibility == VisibilityCompanyConfidential
}

// Lifecycle states.
//
// The last four are not error cases. STALE, GAP_DETECTED, RECONCILING and
// UNKNOWN each mean "nobody can currently vouch for this document", and every
// gate treats them exactly as it treats UNSAFE: it blocks.
const (
	StateUploadPending       = "UPLOAD_PENDING"
	StateUploaded            = "UPLOADED"
	StateValidationPending   = "VALIDATION_PENDING"
	StateSafe                = "SAFE"
	StateUnsafe              = "UNSAFE"
	StateQuarantined         = "QUARANTINED"
	StateVerificationPending = "VERIFICATION_PENDING"
	StateVerified            = "VERIFIED"
	StateRejected            = "REJECTED"
	StateExpired             = "EXPIRED"
	StateRevoked             = "REVOKED"
	StateReplaced            = "REPLACED"
	StateDeleted             = "DELETED"
	StateUnknown             = "UNKNOWN"
	StateStale               = "STALE"
	StateGapDetected         = "GAP_DETECTED"
	StateReconciling         = "RECONCILING"
)

// Verification states.
const (
	VerificationPending     = "PENDING"
	VerificationRequested   = "REQUESTED"
	VerificationVerified    = "VERIFIED"
	VerificationRejected    = "REJECTED"
	VerificationInvalidated = "INVALIDATED"
)

// Safety projections, mirroring media's vocabulary exactly. Renaming them here
// would make a contract change look like a local refactor.
const (
	SafetyPending     = "PENDING"
	SafetySafe        = "SAFE"
	SafetyUnsafe      = "UNSAFE"
	SafetyQuarantined = "QUARANTINED"
	SafetyUnknown     = "UNKNOWN"
)

// Media upload projections.
const (
	MediaUploadPending    = "PENDING"
	MediaUploadAuthorized = "AUTHORIZED"
	MediaUploadUploading  = "UPLOADING"
	MediaUploadCompleted  = "COMPLETED"
	MediaUploadFailed     = "FAILED"
	MediaUploadExpired    = "EXPIRED"
	MediaUploadUnknown    = "UNKNOWN"
)

// Media deletion projections.
const (
	MediaDeletionActive  = "ACTIVE"
	MediaDeletionPending = "DELETION_PENDING"
	MediaDeletionDeleted = "DELETED"
	MediaDeletionUnknown = "DELETION_UNKNOWN"
	MediaDeletionFailed  = "DELETION_FAILED"
)

// Projection freshness. Same vocabulary as the eligibility decision sources so
// one reader does not have to learn two spellings of the same idea.
const (
	FreshnessCurrent     = "CURRENT"
	FreshnessStale       = "STALE"
	FreshnessGapDetected = "GAP_DETECTED"
	FreshnessUnknown     = "UNKNOWN"
	FreshnessReconciling = "RECONCILING"
)

// Retention classes.
const (
	RetentionStandard = "STANDARD"
	RetentionExtended = "EXTENDED"
	RetentionLegal    = "LEGAL"
)

// Actor types.
const (
	ActorOrganizationUser = "ORGANIZATION_USER"
	ActorProvider         = "PROVIDER"
	ActorPlatformService  = "PLATFORM_SERVICE"
	ActorSystem           = "SYSTEM"
)

// History actions.
const (
	ActionCreated              = "CREATED"
	ActionUploadCompleted      = "UPLOAD_COMPLETED"
	ActionValidationPassed     = "VALIDATION_PASSED"
	ActionValidationFailed     = "VALIDATION_FAILED"
	ActionSafetyProjected      = "SAFETY_PROJECTED"
	ActionQuarantined          = "QUARANTINED"
	ActionVerificationRequest  = "VERIFICATION_REQUESTED"
	ActionVerified             = "VERIFIED"
	ActionRejected             = "REJECTED"
	ActionVerificationVoided   = "VERIFICATION_INVALIDATED"
	ActionReplaced             = "REPLACED"
	ActionRevoked              = "REVOKED"
	ActionExpired              = "EXPIRED"
	ActionDeletionScheduled    = "DELETION_SCHEDULED"
	ActionDeletionCompleted    = "DELETION_COMPLETED"
	ActionDeletionBlocked      = "DELETION_BLOCKED"
	ActionDeletionFailed       = "DELETION_FAILED"
	ActionDeletionUnknown      = "DELETION_UNKNOWN"
	ActionRetentionUpdated     = "RETENTION_UPDATED"
	ActionLegalHoldApplied     = "LEGAL_HOLD_APPLIED"
	ActionLegalHoldReleased    = "LEGAL_HOLD_RELEASED"
	ActionDownloadAuthorized   = "DOWNLOAD_AUTHORIZED"
	ActionReconciliationOpened = "RECONCILIATION_OPENED"
	ActionReconciled           = "RECONCILED"
	ActionGapDetected          = "GAP_DETECTED"
	ActionStaleDetected        = "STALE_DETECTED"
	ActionArchived             = "ARCHIVED"
)

// Verification decisions.
const (
	DecisionRequested   = "REQUESTED"
	DecisionVerified    = "VERIFIED"
	DecisionRejected    = "REJECTED"
	DecisionInvalidated = "INVALIDATED"
)

// Rejection reason codes. Controlled so a rejected document tells whoever has
// to fix it what to do, and so rejections can be counted.
const (
	RejectIllegible      = "ILLEGIBLE"
	RejectWrongDocument  = "WRONG_DOCUMENT"
	RejectExpired        = "EXPIRED_ON_ISSUE"
	RejectDetailMismatch = "DETAIL_MISMATCH"
	RejectIncomplete     = "INCOMPLETE"
	RejectSuspectedFraud = "SUSPECTED_FRAUD"
	RejectUnsafeContent  = "UNSAFE_CONTENT"
	RejectSourceChanged  = "SOURCE_VERSION_CHANGED"
)

// ValidRejectionReason reports whether a code is one a verifier may use.
func ValidRejectionReason(code string) bool {
	switch code {
	case RejectIllegible, RejectWrongDocument, RejectExpired, RejectDetailMismatch,
		RejectIncomplete, RejectSuspectedFraud, RejectUnsafeContent, RejectSourceChanged:
		return true
	default:
		return false
	}
}

// Reconciliation case reasons.
const (
	ReconcileVersionGap      = "VERSION_GAP"
	ReconcileStaleProjection = "STALE_PROJECTION"
	ReconcileUnknownOutcome  = "UNKNOWN_OUTCOME"
	ReconcileDeletionUnknown = "DELETION_UNKNOWN"
	ReconcileUploadUnknown   = "UPLOAD_UNKNOWN"
	ReconcileSafetyUnknown   = "SAFETY_UNKNOWN"
	ReconcilePayloadConflict = "PAYLOAD_CONFLICT"
)

// Reconciliation case statuses.
const (
	CaseOpen         = "OPEN"
	CaseInProgress   = "IN_PROGRESS"
	CaseResolved     = "RESOLVED"
	CaseDeadLettered = "DEAD_LETTERED"
)

// Deletion request statuses.
const (
	DeletionScheduled    = "SCHEDULED"
	DeletionInProgress   = "IN_PROGRESS"
	DeletionCompleted    = "COMPLETED"
	DeletionBlocked      = "BLOCKED"
	DeletionUnknown      = "UNKNOWN"
	DeletionFailed       = "FAILED"
	DeletionDeadLettered = "DEAD_LETTERED"
)

// Document is one fleet-side document reference.
type Document struct {
	ID             string
	OrganizationID string
	OwnerType      string
	OwnerID        string
	BranchID       *string
	MediaObjectID  string
	Category       string
	DocumentType   string
	IssueDate      *time.Time
	ExpiryDate     *time.Time

	LifecycleState      string
	VerificationState   string
	SafetyProjection    string
	MediaUploadStatus   string
	MediaDeletionState  string
	MediaSourceVersion  int64
	ProjectionFreshness string
	LastMediaEventID    *string
	SyncedAt            *time.Time

	// Authoritative media projections: populated only from media's own
	// events (see ingestion.applyMediaState), never from caller-supplied
	// upload-request input. Empty/zero until media has actually reported
	// them.
	MediaChecksumSHA256 string
	CapturedAt          *time.Time
	SourceTimestamp     *time.Time
	CaptureMethod       string
	DeviceReference     string
	ActorReference      string
	OriginalMimeType    string
	OriginalSizeBytes   int64

	ReplacementDocumentID *string
	ReplacesDocumentID    *string

	RevokedAt      *time.Time
	RevokedReason  string
	RejectedReason string
	DeletedAt      *time.Time

	RetentionClass  string
	RetentionUntil  *time.Time
	LegalHold       bool
	LegalHoldReason string

	Visibility        string
	ArchivedAt        *time.Time
	ArchivedByActorID string

	CreatedByActorType string
	CreatedByActorID   string
	CorrelationID      string
	CausationID        string
	IdempotencyKey     string
	RequestFingerprint string
	Version            int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Verified reports whether a human signed this document off and the sign-off
// still stands.
func (d Document) Verified() bool {
	return d.VerificationState == VerificationVerified && d.LifecycleState == StateVerified
}

// SafetyConfirmed reports whether media actually vouched for the content.
// PENDING and UNKNOWN both answer false, which is the point of keeping them
// distinct from SAFE.
func (d Document) SafetyConfirmed() bool { return d.SafetyProjection == SafetySafe }

// ProjectionCurrent reports whether fleet's copy of media's answer can be
// relied on right now.
func (d Document) ProjectionCurrent() bool { return d.ProjectionFreshness == FreshnessCurrent }

// Expired reports whether the document has passed its stated expiry.
func (d Document) Expired(now time.Time) bool {
	if d.ExpiryDate == nil {
		return false
	}
	return !d.ExpiryDate.After(now)
}

// Archived reports whether the document has been organizationally archived.
// Archival is orthogonal to the lifecycle state machine: an archived
// document keeps whatever lifecycle/verification state it had, it is simply
// no longer surfaced in default listings — historical file references
// remain reachable by ID after a subject is completed or archived, they are
// just out of the way.
func (d Document) Archived() bool { return d.ArchivedAt != nil }

// Terminal reports whether the document has reached a state no further
// business action can move it out of.
func (d Document) Terminal() bool {
	switch d.LifecycleState {
	case StateRevoked, StateReplaced, StateDeleted:
		return true
	default:
		return false
	}
}

// Verifiable reports whether the document may be signed off right now, and why
// not when it may not.
//
// The single rule: only a SAFE file can become VERIFIED. PENDING, UNKNOWN,
// UNSAFE and QUARANTINED all refuse, because each of them means the platform
// does not actually know the content is clean — and a verification recorded
// against a file nobody vouched for is exactly the evidence an auditor would
// later find worthless.
func (d Document) Verifiable(now time.Time) (bool, string) {
	switch {
	case d.Terminal():
		return false, "document is " + d.LifecycleState
	case d.MediaDeletionState != MediaDeletionActive:
		return false, "media object is not active"
	case d.MediaUploadStatus != MediaUploadCompleted:
		return false, "upload is not complete"
	case !d.SafetyConfirmed():
		return false, "safety status is " + d.SafetyProjection
	case !d.ProjectionCurrent():
		return false, "media projection is " + d.ProjectionFreshness
	case d.Expired(now):
		return false, "document has expired"
	default:
		return true, ""
	}
}

// Downloadable reports whether fleet may ask media for a content grant.
//
// This is fleet's half of the decision. Media applies its own floor on top and
// may still refuse; a document that passes here and is refused there is
// working exactly as intended.
func (d Document) Downloadable() bool {
	switch d.LifecycleState {
	case StateUnsafe, StateQuarantined, StateDeleted, StateUnknown, StateStale,
		StateGapDetected, StateReconciling, StateRevoked:
		return false
	}
	switch d.SafetyProjection {
	case SafetyUnsafe, SafetyQuarantined:
		return false
	}
	if d.MediaDeletionState != MediaDeletionActive {
		return false
	}
	if !d.ProjectionCurrent() {
		return false
	}
	// A replaced document stays readable only while it has not also been
	// revoked or deleted: the audit trail of what was superseded is worth
	// keeping reachable, and the replacement chain says which is current.
	return true
}

// SatisfiesRequirement reports whether this document may satisfy a mandatory
// eligibility requirement, and the controlled reason when it may not.
//
// requireLatest refuses a REPLACED document, for requirements where only the
// current version counts.
func (d Document) SatisfiesRequirement(now time.Time, requireLatest bool) (bool, string) {
	switch {
	case d.LifecycleState == StateDeleted || d.DeletedAt != nil:
		return false, ReasonDocumentDeleted
	case d.LifecycleState == StateRevoked || d.RevokedAt != nil:
		return false, ReasonDocumentRevoked
	case requireLatest && d.LifecycleState == StateReplaced:
		return false, ReasonDocumentReplaced
	case d.MediaDeletionState != MediaDeletionActive:
		return false, ReasonDocumentDeleted
	case !d.SafetyConfirmed():
		return false, ReasonDocumentNotSafe
	case !d.Verified():
		return false, ReasonDocumentNotVerified
	case d.Expired(now):
		return false, ReasonDocumentExpired
	case !d.ProjectionCurrent():
		if d.ProjectionFreshness == FreshnessReconciling ||
			d.ProjectionFreshness == FreshnessGapDetected {
			return false, ReasonDocumentReconciling
		}
		return false, ReasonDocumentProjectionStale
	default:
		return true, ""
	}
}

// Eligibility reason codes. Every value here exists as a row in
// fleet_partner_eligibility_reason_codes, and the foreign key means a typo
// fails the write rather than producing an unfilterable reason.
const (
	ReasonDocumentMissing         = "document_missing"
	ReasonDocumentNotVerified     = "document_not_verified"
	ReasonDocumentNotSafe         = "document_not_safe"
	ReasonDocumentExpired         = "document_expired"
	ReasonDocumentRevoked         = "document_revoked"
	ReasonDocumentReplaced        = "document_replaced"
	ReasonDocumentDeleted         = "document_deleted"
	ReasonDocumentProjectionStale = "document_projection_stale"
	ReasonDocumentReconciling     = "document_reconciling"
)

// Access audit actions and results.
const (
	AccessActionList            = "LIST"
	AccessActionRead            = "READ"
	AccessActionDownloadRequest = "DOWNLOAD_REQUEST"
	AccessResultAllowed         = "ALLOWED"
	AccessResultDenied          = "DENIED"
	AccessSubjectProvider       = "PROVIDER"
	AccessSubjectCustomer       = "CUSTOMER"
)

// AccessAuditEntry is one append-only record of a provider- or
// customer-facing document access attempt, allowed or denied.
type AccessAuditEntry struct {
	ID               string
	OrganizationID   string
	DocumentID       string
	ActorType        string
	ActorReference   string
	SubjectType      string
	SubjectReference string
	Visibility       string
	Action           string
	Result           string
	DenialReasonCode string
	CorrelationID    string
	OccurredAt       time.Time
}

// HistoryEntry is one append-only record of what happened to a document.
type HistoryEntry struct {
	ID             string
	OrganizationID string
	DocumentID     string
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

// VerificationDecision is one immutable verification record.
type VerificationDecision struct {
	ID                  string
	OrganizationID      string
	DocumentID          string
	Decision            string
	ReasonCode          string
	ReasonNote          string
	MediaSourceVersion  int64
	MediaChecksumSHA256 string
	DecidedByActorType  string
	DecidedByActorID    string
	DocumentVersion     int64
	CorrelationID       string
	CausationID         string
	DecidedAt           time.Time
}

// ReconciliationCase is one durable disagreement awaiting an authoritative
// read-back.
type ReconciliationCase struct {
	ID             string
	OrganizationID string
	DocumentID     string
	MediaObjectID  string
	Reason         string
	Status         string
	Attempts       int
	LastErrorCode  string
	NextAttemptAt  time.Time
	LeaseOwner     *string
	LeaseExpiresAt *time.Time
	ResolvedAt     *time.Time
	CorrelationID  string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// DeletionRequest is one durable fleet-side deletion schedule.
type DeletionRequest struct {
	ID                 string
	OrganizationID     string
	DocumentID         string
	MediaObjectID      string
	Reason             string
	Status             string
	BlockReason        string
	Attempts           int
	LastErrorCode      string
	ScheduledFor       time.Time
	NextAttemptAt      time.Time
	LeaseOwner         *string
	LeaseExpiresAt     *time.Time
	RequestedBy        string
	IdempotencyKey     string
	RequestFingerprint string
	CorrelationID      string
	CausationID        string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	CompletedAt        *time.Time
}

// LifecycleForMedia maps media's authoritative answer onto fleet's lifecycle.
//
// It is deliberately one function rather than scattered branches: the mapping
// from another service's vocabulary onto ours is a contract, and a contract
// implemented in six places drifts.
//
// Verification is NOT decided here. A document that was VERIFIED and whose
// file is still SAFE and CURRENT keeps its verified lifecycle; everything else
// falls back to what media says about the file.
func LifecycleForMedia(uploadStatus string, safety string, deletion string,
	freshness string, verified bool) string {
	switch deletion {
	case MediaDeletionDeleted:
		return StateDeleted
	case MediaDeletionUnknown, MediaDeletionFailed:
		return StateUnknown
	}
	switch safety {
	case SafetyUnsafe:
		return StateUnsafe
	case SafetyQuarantined:
		return StateQuarantined
	}
	switch freshness {
	case FreshnessGapDetected:
		return StateGapDetected
	case FreshnessReconciling:
		return StateReconciling
	case FreshnessStale:
		return StateStale
	case FreshnessUnknown:
		return StateUnknown
	}
	switch uploadStatus {
	case MediaUploadPending, MediaUploadAuthorized:
		return StateUploadPending
	case MediaUploadUploading:
		return StateValidationPending
	case MediaUploadFailed:
		return StateRejected
	case MediaUploadExpired:
		return StateExpired
	case MediaUploadUnknown:
		return StateUnknown
	}
	// Upload is COMPLETED from here on.
	if verified && safety == SafetySafe {
		return StateVerified
	}
	switch safety {
	case SafetySafe:
		return StateSafe
	case SafetyPending:
		return StateValidationPending
	default:
		return StateUnknown
	}
}

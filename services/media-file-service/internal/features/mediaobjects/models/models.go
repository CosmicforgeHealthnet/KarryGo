// Package models holds the authoritative media object domain.
//
// media-file-service is the only service that knows whether a file exists,
// what is actually in it, whether anything vouched for it and whether the
// bytes are still there. Every other service holds a reference and a
// projection of that answer; none of them may contradict it.
//
// The three state machines are deliberately independent. An upload can be
// COMPLETED while safety is still PENDING, and a file can be SAFE while its
// deletion is DELETION_UNKNOWN. Collapsing them into one status is what
// produces the failure this package exists to prevent: an unscanned file
// reading as usable because the upload finished.
package models

import "time"

// Upload states.
const (
	UploadPending    = "PENDING"
	UploadAuthorized = "AUTHORIZED"
	UploadUploading  = "UPLOADING"
	UploadCompleted  = "COMPLETED"
	UploadFailed     = "FAILED"
	UploadExpired    = "EXPIRED"
)

// Safety states.
//
// There is no state meaning "assumed safe". SafetyPending is what a file holds
// before anything looked at it; SafetyUnknown is what it holds when something
// tried and could not answer. Both block wherever policy requires SAFE.
const (
	SafetyPending     = "PENDING"
	SafetySafe        = "SAFE"
	SafetyUnsafe      = "UNSAFE"
	SafetyQuarantined = "QUARANTINED"
	SafetyUnknown     = "UNKNOWN"
)

// Deletion states.
const (
	DeletionActive  = "ACTIVE"
	DeletionPending = "DELETION_PENDING"
	DeletionDeleted = "DELETED"
	DeletionUnknown = "DELETION_UNKNOWN"
	DeletionFailed  = "DELETION_FAILED"
)

// Signature validation outcomes.
const (
	SignaturePending     = "PENDING"
	SignaturePassed      = "PASSED"
	SignatureFailed      = "FAILED"
	SignatureUnsupported = "UNSUPPORTED"
)

// Upload authorization states.
const (
	AuthorizationIssued   = "ISSUED"
	AuthorizationConsumed = "CONSUMED"
	AuthorizationExpired  = "EXPIRED"
	AuthorizationFailed   = "FAILED"
)

// Upload purposes this service accepts on the private contract.
const (
	PurposeCompanyDocument    = "COMPANY_DOCUMENT"
	PurposeBranchDocument     = "BRANCH_DOCUMENT"
	PurposeProviderDocument   = "PROVIDER_DOCUMENT"
	PurposeVehicleDocument    = "VEHICLE_DOCUMENT"
	PurposeInsuranceDocument  = "INSURANCE_DOCUMENT"
	PurposeComplianceDocument = "COMPLIANCE_DOCUMENT"
	PurposeProofDocument      = "PROOF_DOCUMENT"
	PurposeIncidentAttachment = "INCIDENT_ATTACHMENT"
	PurposeDisputeAttachment  = "DISPUTE_ATTACHMENT"
	PurposeApprovalAttachment = "APPROVAL_ATTACHMENT"
	// PurposeDataExport is a generated, line-oriented data file rather than a
	// document a human uploaded. It is the only purpose for which textual
	// content is accepted, and it is accepted on a positive inspection of the
	// bytes, never on the caller's word — see validation.ValidateExportContent.
	PurposeDataExport = "DATA_EXPORT"
)

// ValidPurpose reports whether a purpose is one the private contract serves.
func ValidPurpose(purpose string) bool {
	switch purpose {
	case PurposeCompanyDocument, PurposeBranchDocument, PurposeProviderDocument,
		PurposeVehicleDocument, PurposeInsuranceDocument, PurposeComplianceDocument,
		PurposeProofDocument, PurposeIncidentAttachment, PurposeDisputeAttachment,
		PurposeApprovalAttachment, PurposeDataExport:
		return true
	default:
		return false
	}
}

// Object visibility.
//
// PUBLIC exists so the rule about DATA_EXPORT can be stated rather than merely
// implied by the current surface having no public path. A generated export is
// customer data in bulk and must never be reachable without an authenticated,
// audited, short-lived grant; writing that down as a value and a constraint
// makes it survive whatever endpoint is added next.
const (
	VisibilityPrivate = "PRIVATE"
	VisibilityPublic  = "PUBLIC"
)

// ValidVisibility reports whether a value is a supported visibility.
func ValidVisibility(value string) bool {
	return value == VisibilityPrivate || value == VisibilityPublic
}

// RequiresPrivateVisibility reports the purposes that may only ever be private.
func RequiresPrivateVisibility(purpose string) bool {
	return purpose == PurposeDataExport
}

// Retention classes.
const (
	RetentionStandard = "STANDARD"
	RetentionLegal    = "LEGAL"
	RetentionExtended = "EXTENDED"
)

// Validation failure codes. Controlled so a consumer can branch on them and an
// operator can filter for them.
const (
	FailureSizeExceeded      = "SIZE_EXCEEDED"
	FailureMIMEMismatch      = "MIME_MISMATCH"
	FailureSignatureMismatch = "SIGNATURE_MISMATCH"
	FailureChecksumMismatch  = "CHECKSUM_MISMATCH"
	FailureIncompleteUpload  = "INCOMPLETE_UPLOAD"
	FailureUnsupportedType   = "UNSUPPORTED_TYPE"
	FailureExecutableContent = "EXECUTABLE_CONTENT"
	FailureEmptyUpload       = "EMPTY_UPLOAD"
)

// History actions.
const (
	ActionAuthorized        = "UPLOAD_AUTHORIZED"
	ActionUploadCompleted   = "UPLOAD_COMPLETED"
	ActionValidationPassed  = "VALIDATION_PASSED"
	ActionValidationFailed  = "VALIDATION_FAILED"
	ActionScanCompleted     = "SCAN_COMPLETED"
	ActionQuarantined       = "QUARANTINED"
	ActionDownloadAuthorize = "DOWNLOAD_AUTHORIZED"
	ActionDeletionRequested = "DELETION_REQUESTED"
	ActionDeletionCompleted = "DELETION_COMPLETED"
	ActionDeletionFailed    = "DELETION_FAILED"
	ActionDeletionBlocked   = "DELETION_BLOCKED"
	ActionAuthorizationUsed = "AUTHORIZATION_CONSUMED"
)

// Object is one authoritative media object.
type Object struct {
	ID                    string
	OrganizationID        string
	OwnerService          string
	Visibility            string
	FileName              string
	OwnerType             string
	OwnerReference        string
	UploadPurpose         string
	DeclaredMIME          string
	DetectedMIME          string
	SizeBytes             int64
	ChecksumSHA256        string
	CapturedAt            *time.Time
	SourceTimestamp       *time.Time
	CaptureMethod         string
	DeviceReference       string
	ActorReference        string
	OriginalMIMEType      string
	OriginalSizeBytes     int64
	IntegrityHash         string
	SignatureValidation   string
	UploadStatus          string
	SafetyStatus          string
	SafetyReason          string
	ScannedAt             *time.Time
	ValidationFailureCode string
	StorageBucket         string
	StorageObjectKey      string
	DeletionState         string
	DeletionRequestedAt   *time.Time
	DeletedAt             *time.Time
	DeletionFailureCode   string
	RetentionClass        string
	RetentionUntil        *time.Time
	LegalHold             bool
	Version               int64
	CorrelationID         string
	CausationID           string
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// Downloadable reports whether media may issue a content grant for this
// object at all.
//
// This is media's own floor, not the consumer's policy. Fleet applies its
// document rules on top; what is refused here is refused regardless of what
// any consumer believes, because these are facts about the bytes.
func (o Object) Downloadable() bool {
	if o.UploadStatus != UploadCompleted {
		return false
	}
	if o.DeletionState != DeletionActive {
		return false
	}
	switch o.SafetyStatus {
	case SafetyUnsafe, SafetyQuarantined:
		return false
	}
	return true
}

// AccessExpired reports whether a short-lived DATA_EXPORT has passed the
// retention instant supplied by its owning service. Existing document
// purposes keep their established retention semantics; export retention is a
// hard access ceiling as well as a deletion deadline.
func (o Object) AccessExpired(now time.Time) bool {
	return o.UploadPurpose == PurposeDataExport && o.RetentionUntil != nil &&
		!o.RetentionUntil.After(now.UTC())
}

// SafetyConfirmed reports whether something actually vouched for the content.
// PENDING and UNKNOWN both answer false, which is the whole point of keeping
// them distinct from SAFE.
func (o Object) SafetyConfirmed() bool { return o.SafetyStatus == SafetySafe }

// PhysicalDeletionBlocked reports whether retention policy forbids destroying
// the bytes, and why.
func (o Object) PhysicalDeletionBlocked(now time.Time) (bool, string) {
	if o.LegalHold {
		return true, "LEGAL_HOLD"
	}
	if o.RetentionUntil != nil && o.RetentionUntil.After(now) {
		return true, "RETENTION_ACTIVE"
	}
	return false, ""
}

// UploadAuthorization is one single-use grant to upload bytes.
type UploadAuthorization struct {
	ID                 string
	MediaObjectID      string
	OrganizationID     string
	CallerService      string
	OwnerService       string
	OwnerType          string
	OwnerReference     string
	DocumentCategory   string
	UploadPurpose      string
	Visibility         string
	FileName           string
	AllowedMIMETypes   []string
	MaxSizeBytes       int64
	UploadTokenHash    string
	State              string
	ExpiresAt          time.Time
	ConsumedAt         *time.Time
	IdempotencyKey     string
	RequestFingerprint string
	CorrelationID      string
	CausationID        string
	PayloadVersion     int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Usable reports whether this grant may still be redeemed.
func (a UploadAuthorization) Usable(now time.Time) bool {
	return a.State == AuthorizationIssued && a.ExpiresAt.After(now)
}

// AllowsMIME reports whether a content type is inside the grant. An empty
// allowlist means the grant named no types, which is refused rather than
// treated as "anything".
func (a UploadAuthorization) AllowsMIME(contentType string) bool {
	for _, allowed := range a.AllowedMIMETypes {
		if allowed == contentType {
			return true
		}
	}
	return false
}

// HistoryEntry is one append-only record of what happened to an object.
type HistoryEntry struct {
	ID             string
	OrganizationID string
	MediaObjectID  string
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

// DeletionRequest is one durable request to destroy an object's bytes.
type DeletionRequest struct {
	ID                 string
	MediaObjectID      string
	OrganizationID     string
	RequestedBy        string
	Reason             string
	Status             string
	BlockReason        string
	Attempts           int
	LastError          string
	IdempotencyKey     string
	RequestFingerprint string
	CorrelationID      string
	CausationID        string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	CompletedAt        *time.Time
}

// Deletion request statuses.
const (
	DeletionRequestPending   = "PENDING"
	DeletionRequestCompleted = "COMPLETED"
	DeletionRequestBlocked   = "BLOCKED"
	DeletionRequestFailed    = "FAILED"
	DeletionRequestUnknown   = "UNKNOWN"
)

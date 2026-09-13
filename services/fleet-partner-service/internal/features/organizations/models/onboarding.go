package models

import "time"

// Onboarding stages, in the order they are presented.
const (
	StageOwnerContact             = "OWNER_CONTACT"
	StageBusinessProfile          = "BUSINESS_PROFILE"
	StageAuthorizedRepresentative = "AUTHORIZED_REPRESENTATIVE"
	StageOperatingCapabilities    = "OPERATING_CAPABILITIES"
	StageHeadquarters             = "HEADQUARTERS"
	StageBusinessDocuments        = "BUSINESS_DOCUMENTS"
	StageVerification             = "VERIFICATION"
	StageOrganizationSettings     = "ORGANIZATION_SETTINGS"
	StageOperatingTerritories     = "OPERATING_TERRITORIES"
	StageWalletProvisioning       = "WALLET_PROVISIONING"
	StageActivation               = "ACTIVATION"
)

// AllStages lists every onboarding stage in order.
var AllStages = []string{
	StageOwnerContact, StageBusinessProfile, StageAuthorizedRepresentative,
	StageOperatingCapabilities, StageHeadquarters, StageBusinessDocuments,
	StageVerification, StageOrganizationSettings, StageOperatingTerritories,
	StageWalletProvisioning, StageActivation,
}

// Stage statuses.
const (
	StageStatusNotStarted      = "NOT_STARTED"
	StageStatusInProgress      = "IN_PROGRESS"
	StageStatusComplete        = "COMPLETE"
	StageStatusBlocked         = "BLOCKED"
	StageStatusChangesRequired = "CHANGES_REQUIRED"
)

// Organization statuses beyond the Sections 3–7 set.
const (
	OrganizationStatusContactVerified   = "CONTACT_VERIFIED"
	OrganizationStatusBusinessSubmitted = "BUSINESS_SUBMITTED"
	OrganizationStatusUnderReview       = "UNDER_REVIEW"
	OrganizationStatusChangesRequired   = "CHANGES_REQUIRED"
	OrganizationStatusClosed            = "CLOSED"
	OrganizationStatusDraft             = "DRAFT"
)

// Operating capabilities an organization may request.
const (
	CapabilityPackageDelivery    = "PACKAGE_DELIVERY"
	CapabilityTaxi               = "TAXI"
	CapabilityHauling            = "HAULING"
	CapabilityCourierLastMile    = "COURIER_LAST_MILE"
	CapabilityPassengerTransport = "PASSENGER_TRANSPORT"
	CapabilityTrucking           = "TRUCKING"
	CapabilityHeavyHaul          = "HEAVY_HAUL"
	CapabilityFreightForwarding  = "FREIGHT_FORWARDING"
	CapabilityCrossBorder        = "CROSS_BORDER"
	CapabilityColdChain          = "COLD_CHAIN"
	CapabilityMedicalLogistics   = "MEDICAL_LOGISTICS"
	CapabilityAgricultureFood    = "AGRICULTURE_FOOD"
	CapabilityMovingRelocation   = "MOVING_RELOCATION"
	CapabilityTowingRecovery     = "TOWING_RECOVERY"
	CapabilityWarehousing        = "WAREHOUSING"
	CapabilityFulfilment         = "FULFILMENT"
	CapabilityPrivateLogistics   = "PRIVATE_LOGISTICS"
)

// SupportedCapabilities is the full catalogue.
var SupportedCapabilities = []string{
	CapabilityPackageDelivery, CapabilityTaxi, CapabilityHauling, CapabilityCourierLastMile,
	CapabilityPassengerTransport, CapabilityTrucking, CapabilityHeavyHaul,
	CapabilityFreightForwarding, CapabilityCrossBorder, CapabilityColdChain,
	CapabilityMedicalLogistics, CapabilityAgricultureFood, CapabilityMovingRelocation,
	CapabilityTowingRecovery, CapabilityWarehousing, CapabilityFulfilment,
	CapabilityPrivateLogistics,
}

// CapabilitiesRequiringReview must be approved by platform review before the
// organization may operate them.
var CapabilitiesRequiringReview = map[string]bool{
	CapabilityPassengerTransport: true,
	CapabilityTaxi:               true,
	CapabilityHeavyHaul:          true,
	CapabilityCrossBorder:        true,
	CapabilityMedicalLogistics:   true,
	CapabilityColdChain:          true,
}

// Capability statuses.
const (
	CapabilityStatusRequested = "REQUESTED"
	CapabilityStatusApproved  = "APPROVED"
	CapabilityStatusRejected  = "REJECTED"
	CapabilityStatusWithdrawn = "WITHDRAWN"
)

// Wallet provisioning statuses.
const (
	WalletStatusNotRequested           = "NOT_REQUESTED"
	WalletStatusRequested              = "REQUESTED"
	WalletStatusProvisioning           = "PROVISIONING"
	WalletStatusActive                 = "ACTIVE"
	WalletStatusFailedRetryable        = "FAILED_RETRYABLE"
	WalletStatusFailedPermanent        = "FAILED_PERMANENT"
	WalletStatusReconciliationRequired = "RECONCILIATION_REQUIRED"
)

// Verification request statuses.
const (
	VerificationStatusPendingDispatch = "PENDING_DISPATCH"
	VerificationStatusSubmitted       = "SUBMITTED"
	VerificationStatusUnderReview     = "UNDER_REVIEW"
	VerificationStatusApproved        = "APPROVED"
	VerificationStatusChangesRequired = "CHANGES_REQUIRED"
	VerificationStatusRejected        = "REJECTED"
	VerificationStatusUnknownOutcome  = "UNKNOWN_OUTCOME"
)

// Document statuses.
const (
	DocumentStatusAttached = "ATTACHED"
	DocumentStatusReady    = "READY"
	DocumentStatusRejected = "REJECTED"
	DocumentStatusExpired  = "EXPIRED"
	DocumentStatusRemoved  = "REMOVED"
)

var RequiredDocumentTypes = []string{
	"CERTIFICATE_OF_INCORPORATION",
	"TAX_CLEARANCE",
	"INSURANCE_POLICY",
	"OPERATING_LICENSE",
}

// Closure request statuses.
const (
	ClosureStatusRequested        = "REQUESTED"
	ClosureStatusAssessing        = "ASSESSING"
	ClosureStatusBlocked          = "BLOCKED"
	ClosureStatusApprovalRequired = "APPROVAL_REQUIRED"
	ClosureStatusApproved         = "APPROVED"
	ClosureStatusExecuting        = "EXECUTING"
	ClosureStatusCompleted        = "COMPLETED"
	ClosureStatusRejected         = "REJECTED"
	ClosureStatusCancelled        = "CANCELLED"
)

// Closure blocker states. UNKNOWN blocks exactly like BLOCKING: an
// unreachable authoritative service is never read as "no obligation".
const (
	BlockerStateBlocking = "BLOCKING"
	BlockerStateCleared  = "CLEARED"
	BlockerStateUnknown  = "UNKNOWN"
)

// Profile is the extended organization business profile.
type Profile struct {
	OrganizationID      string         `json:"organizationId"`
	BusinessType        *string        `json:"businessType,omitempty"`
	IncorporationDate   *time.Time     `json:"incorporationDate,omitempty"`
	TaxReference        *string        `json:"taxReference,omitempty"`
	RegisteredAddress   map[string]any `json:"registeredAddress,omitempty"`
	OperatingAddress    map[string]any `json:"operatingAddress,omitempty"`
	BusinessEmail       *string        `json:"businessEmail,omitempty"`
	BusinessPhone       *string        `json:"businessPhone,omitempty"`
	Website             *string        `json:"website,omitempty"`
	BusinessDescription *string        `json:"businessDescription,omitempty"`
	Version             int64          `json:"version"`
	CreatedAt           time.Time      `json:"createdAt"`
	UpdatedAt           time.Time      `json:"updatedAt"`
}

// Representative is an authorized representative of the organization.
type Representative struct {
	ID                      string     `json:"id"`
	OrganizationID          string     `json:"organizationId"`
	FullName                string     `json:"fullName"`
	RoleTitle               string     `json:"roleTitle"`
	Email                   string     `json:"email"`
	Phone                   string     `json:"phone"`
	AuthorityType           string     `json:"authorityType"`
	AuthorityDocumentFileID *string    `json:"authorityDocumentFileId,omitempty"`
	IdentityReference       *string    `json:"identityReference,omitempty"`
	IsOnboardingOwner       bool       `json:"isOnboardingOwner"`
	Status                  string     `json:"status"`
	EffectiveFrom           time.Time  `json:"effectiveFrom"`
	EffectiveTo             *time.Time `json:"effectiveTo,omitempty"`
	Version                 int64      `json:"version"`
	CreatedAt               time.Time  `json:"createdAt"`
	UpdatedAt               time.Time  `json:"updatedAt"`
}

// OnboardingStage is one stage's progress.
type OnboardingStage struct {
	OrganizationID string
	Stage          string
	Status         string
	MissingFields  []string
	BlockingIssues []string
	CompletedAt    *time.Time
	UpdatedAt      time.Time
}

// Document references a business document held by media-file-service.
type Document struct {
	ID                    string     `json:"id"`
	OrganizationID        string     `json:"organizationId"`
	DocumentType          string     `json:"documentType"`
	FileID                string     `json:"fileId"`
	VerificationReference *string    `json:"verificationReference,omitempty"`
	Status                string     `json:"status"`
	IssuedAt              *time.Time `json:"issuedAt,omitempty"`
	ExpiresAt             *time.Time `json:"expiresAt,omitempty"`
	SourceService         string     `json:"sourceService"`
	SourceVersion         int64      `json:"sourceVersion"`
	LastEventID           *string    `json:"lastEventId,omitempty"`
	SyncedAt              *time.Time `json:"syncedAt,omitempty"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             time.Time  `json:"updatedAt"`
}

// VerificationRequest is an immutable submission snapshot plus its
// externally-owned outcome projection.
type VerificationRequest struct {
	ID                 string
	OrganizationID     string
	SubmissionVersion  int
	SubmissionSnapshot map[string]any
	Status             string
	ExternalReference  *string
	SourceService      string
	SourceVersion      int64
	LastEventID        *string
	SyncedAt           *time.Time
	SubmittedBy        string
	RequestID          string
	CorrelationID      string
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// VerificationIssue is one append-only review finding.
type VerificationIssue struct {
	ID                    string
	OrganizationID        string
	VerificationRequestID string
	IssueCode             string
	FieldReference        *string
	DocumentReference     *string
	Remediation           string
	RaisedAt              time.Time
}

// WalletProvisioning projects organization wallet state. No balance is ever
// stored: payment-wallet-service owns the ledger.
type WalletProvisioning struct {
	OrganizationID  string
	Status          string
	WalletAccountID *string
	IdempotencyKey  *string
	Attempts        int
	LastErrorCode   *string
	NextAttemptAt   *time.Time
	SourceService   string
	SourceVersion   int64
	LastEventID     *string
	SyncedAt        *time.Time
	Version         int64
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// ClosureRequest is a controlled organization closure workflow.
type ClosureRequest struct {
	ID                string
	OrganizationID    string
	Status            string
	ReasonCode        string
	ReasonText        *string
	RequestedBy       string
	ApprovalRequestID *string
	AssessedAt        *time.Time
	CompletedAt       *time.Time
	Version           int64
	RequestID         string
	CorrelationID     string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// ClosureBlocker is one obligation preventing closure.
type ClosureBlocker struct {
	ID               string    `json:"id"`
	OrganizationID   string    `json:"organizationId"`
	ClosureRequestID string    `json:"closureRequestId"`
	BlockerCode      string    `json:"blockerCode"`
	OwningService    string    `json:"owningService"`
	Detail           string    `json:"detail"`
	State            string    `json:"state"`
	CheckedAt        time.Time `json:"checkedAt"`
}
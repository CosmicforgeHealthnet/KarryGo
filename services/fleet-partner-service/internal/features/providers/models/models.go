// Package models holds organization-managed provider domain models.
//
// Provider identity, authentication and platform status are owned by the
// driver services. Everything here is the organization's relationship with a
// provider, plus opaque references and projections.
package models

import "time"

// Membership statuses.
const (
	StatusInvited                   = "INVITED"
	StatusPendingProviderAcceptance = "PENDING_PROVIDER_ACCEPTANCE"
	StatusPendingSetup              = "PENDING_SETUP"
	StatusActive                    = "ACTIVE"
	StatusDeclined                  = "DECLINED"
	StatusExpired                   = "EXPIRED"
	StatusCancelled                 = "CANCELLED"
	StatusSuspended                 = "SUSPENDED"
	StatusEnding                    = "ENDING"
	StatusRemoved                   = "REMOVED"
)

// Service categories and their owning services.
const (
	CategoryDispatch = "DISPATCH"
	CategoryTaxi     = "TAXI"
	CategoryHauling  = "HAULING"
)

// Onboarding sources.
const (
	SourceSponsored = "ORGANIZATION_SPONSORED"
	SourceLinked    = "EXISTING_PROVIDER_LINKED"
)

// Relationship types.
const (
	RelationshipEmployee   = "EMPLOYEE"
	RelationshipContractor = "CONTRACTOR"
	RelationshipPartner    = "PARTNER"
)

// Invitation statuses.
const (
	InvitationPending   = "PENDING"
	InvitationAccepted  = "ACCEPTED"
	InvitationDeclined  = "DECLINED"
	InvitationExpired   = "EXPIRED"
	InvitationCancelled = "CANCELLED"
)

// Saga statuses.
const (
	SagaInProgress             = "IN_PROGRESS"
	SagaCompleted              = "COMPLETED"
	SagaBlocked                = "BLOCKED"
	SagaReconciliationRequired = "RECONCILIATION_REQUIRED"
	SagaFailedPermanent        = "FAILED_PERMANENT"
)

// Sub-wallet statuses.
const (
	SubWalletNotRequested           = "NOT_REQUESTED"
	SubWalletRequested              = "REQUESTED"
	SubWalletActive                 = "ACTIVE"
	SubWalletFailedRetryable        = "FAILED_RETRYABLE"
	SubWalletFailedPermanent        = "FAILED_PERMANENT"
	SubWalletReconciliationRequired = "RECONCILIATION_REQUIRED"
	SubWalletDisabled               = "DISABLED"
)

// Membership is an organization's relationship with a provider.
type Membership struct {
	ID                    string
	OrganizationID        string
	ProviderService       string
	ServiceCategory       string
	ProviderID            *string
	ProviderMode          string
	OnboardingSource      string
	RelationshipType      string
	EmploymentType        *string
	EmployeeNumber        *string
	HomeBranchID          *string
	Status                string
	AssociationAcceptedAt *time.Time
	JoinedAt              *time.Time
	SuspendedAt           *time.Time
	EndingAt              *time.Time
	RemovedAt             *time.Time
	SourceVersion         int64
	LastEventID           *string
	SyncedAt              *time.Time
	Version               int64
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// Live reports whether the membership still occupies the provider's single
// settlement slot for its service category.
func (m Membership) Live() bool {
	switch m.Status {
	case StatusPendingProviderAcceptance, StatusPendingSetup, StatusActive, StatusSuspended, StatusEnding:
		return true
	default:
		return false
	}
}

// Invitation is a provider association invitation. Only the token hash is
// stored, and it never carries a password or temporary credential.
type Invitation struct {
	ID                    string
	OrganizationID        string
	MembershipID          *string
	ProviderService       string
	LegalName             string
	EmailNormalized       string
	PhoneE164             string
	ExistingProviderID    *string
	InvitationTokenHash   string
	Status                string
	ExpiresAt             time.Time
	DeliveryStatus        string
	NotificationReference *string
	Attempts              int
	CreatedBy             string
	AcceptedAt            *time.Time
	DeclinedAt            *time.Time
	ConsumedAt            *time.Time
	Version               int64
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

// Capability is an organization-controlled provider capability set.
type Capability struct {
	ID             string
	OrganizationID string
	MembershipID   string
	ServiceType    string
	VehicleClasses []string
	OperatingZones []string
	Qualifications []string
	ValidFrom      time.Time
	ValidUntil     *time.Time
	Status         string
	Version        int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// Policy is the organization's assignment policy for a provider.
//
// ProviderDeclineRequired is a platform invariant fixed to true: a managed
// provider always keeps the right to decline offered work. The database
// enforces it with a CHECK constraint as well.
type Policy struct {
	MembershipID            string
	OrganizationID          string
	AssignmentMode          string
	ProviderDeclineRequired bool
	VisibilityPolicy        string
	ShiftRestrictions       map[string]any
	ZoneRestrictions        map[string]any
	WorkloadRestrictions    map[string]any
	Version                 int64
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

// ProvisioningSaga tracks multi-step provider provisioning. A partially
// provisioned provider is never treated as active.
type ProvisioningSaga struct {
	ID                       string
	OrganizationID           string
	MembershipID             string
	ProviderService          string
	ProviderResolutionStatus string
	ProviderIdentityStatus   string
	AssociationStatus        string
	SubWalletStatus          string
	NotificationStatus       string
	OverallStatus            string
	IdempotencyKey           string
	Attempts                 int
	LastErrorCode            *string
	NextAttemptAt            *time.Time
	Version                  int64
	CreatedAt                time.Time
	UpdatedAt                time.Time
}

// SubWalletLink projects the provider sub-wallet identity. It never stores a
// balance: payment-wallet-service owns the ledger.
type SubWalletLink struct {
	ProviderMembershipID string
	OrganizationID       string
	WalletAccountID      *string
	Status               string
	IdempotencyKey       *string
	SourceService        string
	SourceVersion        int64
	LastEventID          *string
	SyncedAt             *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// HistoryEntry is one append-only membership transition.
type HistoryEntry struct {
	ID             string
	OrganizationID string
	MembershipID   string
	FromStatus     string
	ToStatus       string
	ActorType      string
	ActorID        string
	ReasonCode     string
	ReasonText     *string
	RequestID      string
	CorrelationID  string
	OccurredAt     time.Time
}

// AllowedTransitions defines the managed-provider membership state machine.
var AllowedTransitions = map[string]map[string]bool{
	StatusInvited: {
		StatusPendingProviderAcceptance: true,
		StatusDeclined:                  true,
		StatusExpired:                   true,
		StatusCancelled:                 true,
	},
	StatusPendingProviderAcceptance: {
		StatusPendingSetup: true,
		StatusDeclined:     true,
		StatusExpired:      true,
		StatusCancelled:    true,
	},
	StatusPendingSetup: {
		StatusActive:    true,
		StatusCancelled: true,
	},
	StatusActive: {
		StatusSuspended: true,
		StatusEnding:    true,
	},
	StatusSuspended: {
		StatusActive: true,
		StatusEnding: true,
	},
	StatusEnding:    {StatusRemoved: true},
	StatusDeclined:  {},
	StatusExpired:   {},
	StatusCancelled: {},
	StatusRemoved:   {},
}

// CanTransition reports whether a membership move is allowed.
func CanTransition(from string, to string) bool {
	next, ok := AllowedTransitions[from]
	if !ok {
		return false
	}
	return next[to]
}

// Package models holds the foundational organization domain models.
//
// Domain models are infrastructure-free: no SQL drivers, no HTTP routers,
// no provider SDKs (enforced by architecture tests).
package models

import "time"

// Organization statuses.
const (
	OrganizationStatusPendingReview = "PENDING_REVIEW"
	OrganizationStatusActive        = "ACTIVE"
	OrganizationStatusSuspended     = "SUSPENDED"
	OrganizationStatusArchived      = "ARCHIVED"
)

// Operating modes control how an organization participates in corporate work.
const (
	OperatingModeCentralized = "CENTRALIZED"
	OperatingModeDelegated   = "DELEGATED"
	OperatingModeHybrid      = "HYBRID"
)

// ValidOperatingMode reports whether mode is one of the persisted values.
func ValidOperatingMode(mode string) bool {
	return mode == OperatingModeCentralized || mode == OperatingModeDelegated || mode == OperatingModeHybrid
}

// Branch and department statuses.
const (
	BranchStatusActive   = "ACTIVE"
	BranchStatusInactive = "INACTIVE"
	BranchStatusArchived = "ARCHIVED"

	DepartmentStatusActive   = "ACTIVE"
	DepartmentStatusArchived = "ARCHIVED"
)

// Organization is the top-level logistics-company tenant.
type Organization struct {
	ID                    string
	Status                string
	LegalName             string
	TradingName           string
	RegistrationNumber    string
	CountryCode           string
	Timezone              string
	DefaultCurrency       string
	OperatingCapabilities []string
	Version               int64
	CreatedAt             time.Time
	UpdatedAt             time.Time
	ArchivedAt            *time.Time
}

// Branch is an operational branch, hub, depot, warehouse or office.
type Branch struct {
	ID                string
	OrganizationID    string
	Code              string
	Name              string
	Type              string
	Address           string
	Latitude          *float64
	Longitude         *float64
	GeofenceReference string
	OperatingHours    string
	Timezone          string
	Status            string
	Version           int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
	ArchivedAt        *time.Time
}

// Department belongs to an organization, optionally to a branch and parent.
type Department struct {
	ID             string
	OrganizationID string
	BranchID       *string
	ParentID       *string
	CostCentreID   *string
	Code           string
	Name           string
	Status         string
	Version        int64
	CreatedAt      time.Time
	UpdatedAt      time.Time
	ArchivedAt     *time.Time
}

// Settings is the single current settings record of an organization.
type Settings struct {
	OrganizationID            string
	AssignmentTimeoutSeconds  int
	OperatingMode             string
	ApprovalThresholds        map[string]any
	ProviderVisibility        string
	FinancialVisibilityPolicy map[string]bool
	RetentionSettings         map[string]any
	FeatureFlags              map[string]bool
	Version                   int64
	CreatedAt                 time.Time
	UpdatedAt                 time.Time
}

// StatusChange is one append-only organization status transition.
type StatusChange struct {
	ID              string
	OrganizationID  string
	PreviousStatus  string
	NewStatus       string
	ActorType       string
	ActorID         string
	ReasonCode      string
	ReasonText      string
	EvidenceFileIDs []string
	RequestID       string
	CorrelationID   string
	OccurredAt      time.Time
}

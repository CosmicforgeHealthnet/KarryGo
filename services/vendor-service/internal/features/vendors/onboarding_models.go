package vendors

import (
	"context"
	"time"
)

type VerificationDimension string

const (
	DimensionIdentity    VerificationDimension = "IDENTITY"
	DimensionBusiness    VerificationDimension = "BUSINESS"
	DimensionBank        VerificationDimension = "BANK"
	DimensionLocation    VerificationDimension = "LOCATION"
	DimensionBrand       VerificationDimension = "BRAND"
	DimensionFoodKitchen VerificationDimension = "FOOD_KITCHEN"
)

var VerificationDimensions = []VerificationDimension{
	DimensionIdentity, DimensionBusiness, DimensionBank, DimensionLocation, DimensionBrand, DimensionFoodKitchen,
}

func validDimension(v VerificationDimension) bool {
	for _, candidate := range VerificationDimensions {
		if v == candidate {
			return true
		}
	}
	return false
}

type Representative struct {
	ID               string    `json:"id"`
	BusinessID       string    `json:"business_id"`
	StaffRef         string    `json:"staff_ref"`
	Capacity         string    `json:"capacity"`
	FirstName        string    `json:"first_name"`
	LastName         string    `json:"last_name"`
	Email            string    `json:"email"`
	Phone            string    `json:"phone"`
	OwnershipPercent *float64  `json:"ownership_percent,omitempty"`
	Authorized       bool      `json:"authorized"`
	Active           bool      `json:"active"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	Version          int64     `json:"version"`
}

type OnboardingDocument struct {
	ID               string                `json:"id"`
	BusinessID       string                `json:"business_id"`
	RepresentativeID string                `json:"representative_id,omitempty"`
	BranchID         string                `json:"branch_id,omitempty"`
	BrandID          string                `json:"brand_id,omitempty"`
	Dimension        VerificationDimension `json:"dimension"`
	MediaRef         string                `json:"media_ref"`
	DocumentType     string                `json:"document_type"`
	Status           string                `json:"status"`
	CreatedBy        string                `json:"created_by"`
	CreatedAt        time.Time             `json:"created_at"`
	UpdatedAt        time.Time             `json:"updated_at"`
	Version          int64                 `json:"version"`
}

type VerificationProjection struct {
	ID            string                `json:"id"`
	BusinessID    string                `json:"business_id"`
	Dimension     VerificationDimension `json:"dimension"`
	SubjectType   string                `json:"subject_type"`
	SubjectRef    string                `json:"subject_ref"`
	SourceService string                `json:"source_service"`
	SourceRef     string                `json:"source_ref"`
	SourceVersion int64                 `json:"source_version"`
	Status        VerificationState     `json:"status"`
	ObservedAt    time.Time             `json:"observed_at"`
	ExpiresAt     *time.Time            `json:"expires_at,omitempty"`
	ReasonCode    string                `json:"reason_code,omitempty"`
	Remediation   string                `json:"remediation,omitempty"`
	UpdatedAt     time.Time             `json:"updated_at"`
	Version       int64                 `json:"version"`
}

type RequirementResult struct {
	Code           string                `json:"code"`
	Scope          string                `json:"scope"`
	Dimension      VerificationDimension `json:"dimension,omitempty"`
	CapabilityType CapabilityType        `json:"capability_type,omitempty"`
	SubjectRef     string                `json:"subject_ref,omitempty"`
	Satisfied      bool                  `json:"satisfied"`
	Status         string                `json:"status"`
	Blocker        string                `json:"blocker,omitempty"`
}

type ActivationReadiness struct {
	Ready        bool                `json:"ready"`
	Progress     int                 `json:"progress_percent"`
	Requirements []RequirementResult `json:"requirements"`
	Blockers     []RequirementResult `json:"blockers"`
}

type OnboardingOverview struct {
	Status         string                   `json:"status"`
	Version        int64                    `json:"version"`
	Representative *Representative          `json:"representative,omitempty"`
	Verifications  []VerificationProjection `json:"verifications"`
	Readiness      ActivationReadiness      `json:"activation_readiness"`
}

type BankProfile struct {
	BusinessID          string            `json:"business_id"`
	ExternalRef         string            `json:"external_ref"`
	BankCode            string            `json:"bank_code"`
	MaskedAccount       string            `json:"masked_account"`
	ResolvedAccountName string            `json:"resolved_account_name"`
	SourceVersion       int64             `json:"source_version"`
	Status              VerificationState `json:"status"`
	ObservedAt          time.Time         `json:"observed_at"`
	UpdatedAt           time.Time         `json:"updated_at"`
	Version             int64             `json:"version"`
}

type VerificationCaseInput struct {
	ClientReference string
	BusinessID      string
	Dimension       VerificationDimension
	SubjectType     string
	SubjectRef      string
	DocumentRefs    []string
}

type VerificationCaseResult struct {
	SourceRef     string
	SourceVersion int64
	Status        VerificationState
	ObservedAt    time.Time
	ExpiresAt     *time.Time
	ReasonCode    string
	Remediation   string
}

type VerificationAuthority interface {
	CreateCase(ctx context.Context, input VerificationCaseInput) (VerificationCaseResult, error)
	GetCaseByClientReference(ctx context.Context, clientReference string) (VerificationCaseResult, error)
}

type BankAuthorityResult struct {
	ExternalRef         string
	BankCode            string
	MaskedAccount       string
	ResolvedAccountName string
	SourceVersion       int64
	Status              VerificationState
	ObservedAt          time.Time
}

type BankAuthority interface {
	ResolveAndRegister(ctx context.Context, businessID, accountNumber, bankCode, idempotencyKey string) (BankAuthorityResult, error)
	GetProfile(ctx context.Context, businessID string) (BankAuthorityResult, error)
}

type MediaObject struct {
	ID             string
	OwnerReference string
	UploadPurpose  string
	UploadStatus   string
	SafetyStatus   string
	DeletionState  string
	SourceVersion  int64
}

type MediaAuthority interface {
	GetObject(ctx context.Context, businessID, mediaRef string) (MediaObject, error)
}

type StepUpConsumer interface {
	ConsumeGrant(ctx context.Context, grantID, sessionID, staffID, vendorID, purpose string) error
}

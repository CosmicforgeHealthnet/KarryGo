package verification

import "time"

type Step string

const (
	StepIdentity  Step = "identity"
	StepLicence   Step = "licence"
	StepVehicle   Step = "vehicle"
	StepFace      Step = "face"
	StepGuarantor Step = "guarantor"
	StepEmergency Step = "emergency"
	// StepAll is used only in verification_audit rows that record provider-level
	// actions (e.g. fully_approved).  It is not a submittable verification step.
	StepAll Step = "all"
)

type StepStatus string

const (
	StatusPending   StepStatus = "pending"
	StatusSubmitted StepStatus = "submitted"
	StatusApproved  StepStatus = "approved"
	StatusRejected  StepStatus = "rejected"
)

type ConfirmMethod string

const (
	ConfirmManual ConfirmMethod = "manual"
	ConfirmAuto   ConfirmMethod = "auto"
)

type AuditAction string

const (
	AuditActionSubmitted     AuditAction = "submitted"
	AuditActionApproved      AuditAction = "approved"
	AuditActionRejected      AuditAction = "rejected"
	AuditActionResubmitted   AuditAction = "resubmitted"
	AuditActionAutoConfirmed AuditAction = "auto_confirmed"
	AuditActionSuspended     AuditAction = "suspended"
	AuditActionFaceFailed    AuditAction = "face_failed"
	AuditActionFullyApproved AuditAction = "fully_approved"
)

const RolePlatformAdmin = "platform_admin"

type OverallStatus string

const (
	OverallStatusNotStarted    OverallStatus = "not_started"
	OverallStatusInProgress    OverallStatus = "in_progress"
	OverallStatusPendingReview OverallStatus = "pending_review"
	OverallStatusVerified      OverallStatus = "verified"
	OverallStatusRejected      OverallStatus = "rejected"
	OverallStatusSuspended     OverallStatus = "suspended"
)

type VerificationStep struct {
	ID              string         `json:"id"`
	ProviderID      string         `json:"provider_id"`
	Step            Step           `json:"step"`
	IsOptional      bool           `json:"is_optional"`
	IsAutoConfirmed bool           `json:"is_auto_confirmed"`
	ConfirmMethod   *ConfirmMethod `json:"confirm_method,omitempty"`
	Status          StepStatus     `json:"status"`
	SubmittedAt     *time.Time     `json:"submitted_at,omitempty"`
	ReviewedAt      *time.Time     `json:"reviewed_at,omitempty"`
	ReviewerID      *string        `json:"reviewer_id,omitempty"`
	RejectionReason *string        `json:"rejection_reason,omitempty"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

type VerificationDocument struct {
	ID           string    `json:"id"`
	StepID       string    `json:"step_id"`
	ProviderID   string    `json:"provider_id"`
	DocumentType string    `json:"document_type"`
	FileURL      string    `json:"file_url"`
	FileSize     *int      `json:"file_size,omitempty"`
	MimeType     *string   `json:"mime_type,omitempty"`
	UploadedAt   time.Time `json:"uploaded_at"`
}

type FaceCheck struct {
	ID           string     `json:"id"`
	ProviderID   string     `json:"provider_id"`
	StepID       string     `json:"step_id"`
	SelfieURL    string     `json:"selfie_url"`
	IDDocURL     string     `json:"id_doc_url"`
	MatchScore   *float64   `json:"match_score,omitempty"`
	Result       *string    `json:"result,omitempty"`
	ProviderUsed string     `json:"provider_used"`
	ErrorMessage *string    `json:"error_message,omitempty"`
	CheckedAt    *time.Time `json:"checked_at,omitempty"`
}

type ProviderVerificationState struct {
	ProviderID         string
	VerificationStatus string
	IsActive           bool
}

type VerificationAudit struct {
	ID          string      `json:"id"`
	ProviderID  string      `json:"provider_id"`
	Step        Step        `json:"step"`
	Action      AuditAction `json:"action"`
	FromStatus  StepStatus  `json:"from_status"`
	ToStatus    StepStatus  `json:"to_status"`
	PerformedBy *string     `json:"performed_by,omitempty"`
	Notes       *string     `json:"notes,omitempty"`
	CreatedAt   time.Time   `json:"created_at"`
}

type DocumentInput struct {
	StepID       string
	ProviderID   string
	DocumentType string
	FileURL      string
	FileSize     *int
	MimeType     *string
}

type FaceCheckInput struct {
	ProviderID   string
	StepID       string
	SelfieURL    string
	IDDocURL     string
	MatchScore   *float64
	Result       *string
	ProviderUsed string
	ErrorMessage *string
}

type FaceCheckResultInput struct {
	ID           string
	MatchScore   *float64
	Result       *string
	ErrorMessage *string
}

type AuditInput struct {
	ProviderID  string
	Step        Step
	Action      AuditAction
	FromStatus  StepStatus
	ToStatus    StepStatus
	PerformedBy *string
	Notes       *string
}

type IdentitySubmissionInput struct {
	ProviderID    string
	CorrelationID string
	GovtIDType    string
	GovtIDNumber  string
	GovtIDFile    FileUpload
	ProfilePhoto  FileUpload
}

type LicenceSubmissionInput struct {
	ProviderID    string
	CorrelationID string
	LicenceNumber string
	ExpiryYear    string
	ExpiryMonth   string
	LicenceFile   FileUpload
}

type FaceSubmissionInput struct {
	ProviderID    string
	CorrelationID string
	Selfie        FileUpload
}

type FileUpload struct {
	Header FileHeader
}

type FileHeader interface {
	Open() (File, error)
	GetFilename() string
	GetSize() int64
	GetHeaderValue(key string) string
}

type File interface {
	Read([]byte) (int, error)
	Close() error
}

type StepSubmissionResponse struct {
	Step        Step       `json:"step"`
	Status      StepStatus `json:"status"`
	SubmittedAt time.Time  `json:"submitted_at"`
	Message     string     `json:"message"`
}

type FaceSubmissionResponse struct {
	Step       Step       `json:"step"`
	Result     string     `json:"result"`
	MatchScore float64    `json:"match_score"`
	Status     StepStatus `json:"status"`
	Message    string     `json:"message"`
}

type AdminReviewAction string

const (
	AdminActionApprove AdminReviewAction = "approve"
	AdminActionReject  AdminReviewAction = "reject"
)

type AdminReviewRequest struct {
	Step   Step              `json:"step"`
	Action AdminReviewAction `json:"action"`
	Reason string            `json:"reason"`
}

type AdminReviewResponse struct {
	Step            Step       `json:"step"`
	Status          StepStatus `json:"status"`
	ReviewedAt      *time.Time `json:"reviewed_at,omitempty"`
	ReviewerID      *string    `json:"reviewer_id,omitempty"`
	RejectionReason *string    `json:"rejection_reason,omitempty"`
}

type VerificationStepSummary struct {
	Step        Step       `json:"step"`
	Status      StepStatus `json:"status"`
	IsOptional  bool       `json:"is_optional"`
	SubmittedAt *time.Time `json:"submitted_at"`
	ReviewedAt  *time.Time `json:"reviewed_at"`
}

type LastFaceCheckResponse struct {
	Result     *string    `json:"result"`
	MatchScore *float64   `json:"match_score"`
	CheckedAt  *time.Time `json:"checked_at"`
}

type StepStatusResponse struct {
	ID              string                 `json:"id"`
	ProviderID      string                 `json:"provider_id"`
	Step            Step                   `json:"step"`
	Status          StepStatus             `json:"status"`
	IsOptional      bool                   `json:"is_optional"`
	IsAutoConfirmed bool                   `json:"is_auto_confirmed"`
	ConfirmMethod   *ConfirmMethod         `json:"confirm_method,omitempty"`
	SubmittedAt     *time.Time             `json:"submitted_at"`
	ReviewedAt      *time.Time             `json:"reviewed_at"`
	ReviewerID      *string                `json:"reviewer_id,omitempty"`
	RejectionReason *string                `json:"rejection_reason"`
	UpdatedAt       time.Time              `json:"updated_at"`
	Documents       []VerificationDocument `json:"documents,omitempty"`
	LastFaceCheck   *LastFaceCheckResponse `json:"last_face_check,omitempty"`
}

type AllStatusResponse struct {
	OverallStatus        OverallStatus             `json:"overall_status"`
	CompletionPercentage int                       `json:"completion_percentage"`
	Steps                []VerificationStepSummary `json:"steps"`
}

type InitializationResult struct {
	InsertedSteps          int
	AutoConfirmedAuditRows int
}

func IsValidStep(step Step) bool {
	switch step {
	case StepIdentity, StepLicence, StepVehicle, StepFace, StepGuarantor, StepEmergency:
		return true
	default:
		return false
	}
}

func IsValidStatus(status StepStatus) bool {
	switch status {
	case StatusPending, StatusSubmitted, StatusApproved, StatusRejected:
		return true
	default:
		return false
	}
}

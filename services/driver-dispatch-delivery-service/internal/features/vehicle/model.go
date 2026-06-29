package vehicle

import (
	"net/http"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
)

// ── Type constants ────────────────────────────────────────────────────────────

type BikeType string

const (
	BikeMotorcycle BikeType = "motorcycle"
	BikeDispatch   BikeType = "dispatch_bike"
	BikeScooter    BikeType = "scooter"
	BikeTricycle   BikeType = "tricycle"
	BikeBicycle    BikeType = "bicycle"
	BikeElectric   BikeType = "electric_bike"
)

type VehicleStatus string

const (
	VehicleUnverified VehicleStatus = "unverified"
	VehiclePending    VehicleStatus = "pending"
	VehicleVerified   VehicleStatus = "verified"
	VehicleRejected   VehicleStatus = "rejected"
	VehicleSuspended  VehicleStatus = "suspended"
)

type DocumentType string

const (
	DocRegistration DocumentType = "registration"
	DocInsurance    DocumentType = "insurance"
)

type AuditAction string

const (
	AuditRegistered   AuditAction = "registered"
	AuditDocsUploaded AuditAction = "docs_uploaded"
	AuditApproved     AuditAction = "approved"
	AuditRejected     AuditAction = "rejected"
	AuditSuspended    AuditAction = "suspended"
	AuditResubmitted  AuditAction = "resubmitted"
	AuditUpdated      AuditAction = "updated"
)

const RolePlatformAdmin = "platform_admin"

// ── Domain models ─────────────────────────────────────────────────────────────

type Bike struct {
	ID                 string        `json:"id"`
	ProviderID         string        `json:"provider_id"`
	BikeType           BikeType      `json:"bike_type"`
	Brand              string        `json:"brand"`
	Model              string        `json:"model"`
	Year               int           `json:"year"`
	Color              string        `json:"color"`
	PlateNumber        string        `json:"plate_number"`
	EngineCc           *int          `json:"engine_cc,omitempty"`
	ChassisNumber      *string       `json:"chassis_number,omitempty"`
	VerificationStatus VehicleStatus `json:"verification_status"`
	IsActive           bool          `json:"is_active"`
	IsPrimary          bool          `json:"is_primary"`
	CreatedAt          time.Time     `json:"created_at"`
	UpdatedAt          time.Time     `json:"updated_at"`
}

type BikeDocument struct {
	ID           string       `json:"id"`
	BikeID       string       `json:"bike_id"`
	ProviderID   string       `json:"provider_id"`
	DocumentType DocumentType `json:"document_type"`
	FileURL      string       `json:"file_url"`
	FileSize     *int         `json:"file_size,omitempty"`
	MimeType     *string      `json:"mime_type,omitempty"`
	ExpiryDate   *string      `json:"expiry_date,omitempty"`
	UploadedAt   time.Time    `json:"uploaded_at"`
}

type BikeAudit struct {
	ID          string        `json:"id"`
	BikeID      string        `json:"bike_id"`
	ProviderID  string        `json:"provider_id"`
	Action      AuditAction   `json:"action"`
	FromStatus  VehicleStatus `json:"from_status"`
	ToStatus    VehicleStatus `json:"to_status"`
	PerformedBy *string       `json:"performed_by,omitempty"`
	Notes       *string       `json:"notes,omitempty"`
	CreatedAt   time.Time     `json:"created_at"`
}

// ── Input/output structs ──────────────────────────────────────────────────────

type RegisterBikeInput struct {
	BikeType      BikeType `json:"bike_type"`
	Brand         string   `json:"brand"`
	Model         string   `json:"model"`
	Year          int      `json:"year"`
	Color         string   `json:"color"`
	PlateNumber   string   `json:"plate_number"`
	EngineCc      *int     `json:"engine_cc,omitempty"`
	ChassisNumber *string  `json:"chassis_number,omitempty"`
}

type UpdateBikeInput struct {
	Brand         *string   `json:"brand,omitempty"`
	Model         *string   `json:"model,omitempty"`
	Year          *int      `json:"year,omitempty"`
	Color         *string   `json:"color,omitempty"`
	EngineCc      *int      `json:"engine_cc,omitempty"`
	ChassisNumber *string   `json:"chassis_number,omitempty"`
	BikeType      *BikeType `json:"bike_type,omitempty"`
	PlateNumber   *string   `json:"plate_number,omitempty"`
}

type AdminReviewInput struct {
	Action AuditAction `json:"action"` // approved | rejected | suspended
	Reason string      `json:"reason"`
}

type AuditInput struct {
	BikeID      string
	ProviderID  string
	Action      AuditAction
	FromStatus  VehicleStatus
	ToStatus    VehicleStatus
	PerformedBy *string
	Notes       *string
}

// BikeWithDocuments is the response for GET /provider/vehicle/:id — bike detail
// including an embedded documents slice (never nil, always at least []).
type BikeWithDocuments struct {
	ID                 string         `json:"id"`
	ProviderID         string         `json:"provider_id"`
	BikeType           BikeType       `json:"bike_type"`
	Brand              string         `json:"brand"`
	Model              string         `json:"model"`
	Year               int            `json:"year"`
	Color              string         `json:"color"`
	PlateNumber        string         `json:"plate_number"`
	EngineCc           *int           `json:"engine_cc,omitempty"`
	ChassisNumber      *string        `json:"chassis_number,omitempty"`
	VerificationStatus VehicleStatus  `json:"verification_status"`
	IsActive           bool           `json:"is_active"`
	IsPrimary          bool           `json:"is_primary"`
	CreatedAt          time.Time      `json:"created_at"`
	UpdatedAt          time.Time      `json:"updated_at"`
	Documents          []BikeDocument `json:"documents"`
}

// UploadDocumentInput carries all validated upload data into the service layer.
type UploadDocumentInput struct {
	ProviderID    string
	BikeID        string
	CorrelationID string
	DocumentType  DocumentType
	File          File
	Header        FileHeader
	ExpiryDate    *string // YYYY-MM-DD; required for insurance
}

// Allowed MIME types for bike documents.
const (
	MIMEJPEG = "image/jpeg"
	MIMEPNG  = "image/png"
	MIMEPDF  = "application/pdf"
)

// MaxDocumentSize is 5 MB.
const MaxDocumentSize int64 = 5 * 1024 * 1024

// ── Helpers ───────────────────────────────────────────────────────────────────

func IsValidBikeType(t BikeType) bool {
	return t == BikeMotorcycle || t == BikeDispatch ||
		t == BikeScooter || t == BikeTricycle || t == BikeBicycle || t == BikeElectric
}

func IsValidDocumentType(t DocumentType) bool {
	return t == DocRegistration || t == DocInsurance
}

func IsAllowedMIMEType(mime string) bool {
	return mime == MIMEJPEG || mime == MIMEPNG || mime == MIMEPDF
}

func IsValidAdminAction(a AuditAction) bool {
	return a == AuditApproved || a == AuditRejected || a == AuditSuspended
}

func validationError(message string, fields []apperrors.FieldViolation) *apperrors.Error {
	err := apperrors.New(http.StatusBadRequest, apperrors.CodeValidationFailed, message, nil)
	err.Fields = fields
	return err
}

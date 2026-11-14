// Package models defines asynchronous order-import operations and row outcomes.
package models

import "time"

const (
	StatusUploaded           = "UPLOADED"
	StatusValidating         = "VALIDATING"
	StatusValidationFailed   = "VALIDATION_FAILED"
	StatusReady              = "READY"
	StatusImporting          = "IMPORTING"
	StatusCompleted          = "COMPLETED"
	StatusPartiallyCompleted = "PARTIALLY_COMPLETED"
	StatusFailed             = "FAILED"
	StatusCancelled          = "CANCELLED"
)

// Section 25C: the generic (engine=GENERIC) status vocabulary, additive
// alongside the legacy (engine=LEGACY) one above. The two never mix on one
// row — every generic-engine job uses only these values, every legacy-engine
// job uses only the ones above, and the "pending vs actively claimed" pairing
// mirrors the SKIP LOCKED lease pattern exactly (Pending = queued,
// unsuffixed = currently leased by a worker).
const (
	StatusValidationPending  = "VALIDATION_PENDING"
	StatusValidated          = "VALIDATED"
	StatusCommitPending      = "COMMIT_PENDING"
	StatusCommitting         = "COMMITTING"
	StatusPartiallyCommitted = "PARTIALLY_COMMITTED"
	StatusCommitted          = "COMMITTED"
	StatusDeadLetter         = "DEAD_LETTER"
)

// Import types the generic engine recognizes. PRIVATE_ORDER is also the
// implicit type of every legacy-engine import (the column default).
const (
	TypePrivateOrder       = "PRIVATE_ORDER"
	TypeWorkforceProfile   = "WORKFORCE_PROFILE"
	TypeAsset              = "ASSET"
	TypeWarehouseInventory = "WAREHOUSE_INVENTORY"
)

var validImportTypes = map[string]bool{
	TypePrivateOrder: true, TypeWorkforceProfile: true, TypeAsset: true, TypeWarehouseInventory: true,
}

// ValidImportType reports whether the type is one the database CHECK
// constraint recognizes. A registered Adapter is a separate, additional
// requirement — see the usecases package.
func ValidImportType(importType string) bool { return validImportTypes[importType] }

// Modes.
const (
	ModeDryRun = "DRY_RUN"
	ModeCommit = "COMMIT"
)

func ValidMode(mode string) bool { return mode == ModeDryRun || mode == ModeCommit }

// Engines. Legacy is the column default: every pre-Section-25C import row is
// LEGACY, and Create/RunWorkerPass/Claim/Cancel/Replay in this package's
// usecases and repositories packages read and write ONLY legacy-engine rows,
// completely unaware the GENERIC engine exists. The two engines share the
// same tables but never the same row.
const (
	EngineLegacy  = "LEGACY"
	EngineGeneric = "GENERIC"
)

type Import struct {
	ID                 string     `json:"id"`
	OrganizationID     string     `json:"organizationId"`
	BranchID           string     `json:"branchId"`
	MediaFileID        string     `json:"mediaFileId"`
	FileName           string     `json:"fileName"`
	FileFormat         string     `json:"fileFormat"`
	ContentType        string     `json:"contentType"`
	Status             string     `json:"status"`
	SizeBytes          int64      `json:"sizeBytes"`
	TotalRows          int        `json:"totalRows"`
	ValidRows          int        `json:"validRows"`
	InvalidRows        int        `json:"invalidRows"`
	ImportedRows       int        `json:"importedRows"`
	FailedRows         int        `json:"failedRows"`
	ErrorReportMediaID string     `json:"errorReportMediaId"`
	RequestFingerprint string     `json:"requestFingerprint"`
	Version            int64      `json:"version"`
	CreatedBy          string     `json:"createdBy"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
	CompletedAt        *time.Time `json:"completedAt"`
	CancelledAt        *time.Time `json:"cancelledAt"`
	Attempts           int        `json:"attempts"`

	// Section 25C generic-engine fields. Zero-valued (empty string / 0) on
	// every legacy-engine row.
	ImportType        string `json:"importType"`
	Engine            string `json:"engine"`
	Mode              string `json:"mode"`
	SchemaVersion     int    `json:"schemaVersion"`
	FileFingerprint   string `json:"fileFingerprint"`
	ExternalReference string `json:"externalReference"`
	CommittedRows     int    `json:"committedRows"`
	ValidationVersion int    `json:"validationVersion"`
}

// Terminal reports whether the job has reached a state no further worker
// action can move it out of, in either vocabulary.
func (i Import) Terminal() bool {
	switch i.Status {
	case StatusCompleted, StatusPartiallyCompleted, StatusFailed, StatusCancelled,
		StatusCommitted, StatusPartiallyCommitted, StatusDeadLetter, StatusValidationFailed:
		return true
	default:
		return false
	}
}

type Row struct {
	ID               string    `json:"id"`
	OrganizationID   string    `json:"organizationId"`
	ImportID         string    `json:"importId"`
	RowNumber        int       `json:"rowNumber"`
	RowReference     string    `json:"rowReference"`
	RowFingerprint   string    `json:"rowFingerprint"`
	Status           string    `json:"status"`
	GeneratedOrderID string    `json:"generatedOrderId"`
	SafeErrorCode    string    `json:"safeErrorCode"`
	SafeErrorMessage string    `json:"safeErrorMessage"`
	Data             OrderRow  `json:"data"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`

	// Section 25C generic-engine fields.
	ExternalReference         string `json:"externalReference"`
	SchemaVersion             int    `json:"schemaVersion"`
	CommitStatus              string `json:"commitStatus"`
	GeneratedSubjectReference string `json:"generatedSubjectReference"`
	RowVersion                int    `json:"rowVersion"`
	ReplacesRowID             string `json:"replacesRowId"`
	// GenericData is the normalized row payload for a generic-engine row. The
	// legacy Data field stays typed to OrderRow and is never populated for a
	// generic row — the two engines' rows are read through separate
	// repository methods (ListRows for legacy, ListGenericRows for generic)
	// precisely so one is never accidentally unmarshalled as the other.
	GenericData map[string]any `json:"genericData"`
}

// Row commit statuses (generic engine only).
const (
	RowCommitPending   = "PENDING"
	RowCommitCommitted = "COMMITTED"
	RowCommitFailed    = "FAILED"
	RowCommitSkipped   = "SKIPPED"
)

// Row validation statuses (generic engine; PENDING/COMMITTED/SKIPPED are
// additive alongside the legacy VALID/INVALID/IMPORTED/FAILED vocabulary).
const (
	RowStatusPending   = "PENDING"
	RowStatusValid     = "VALID"
	RowStatusInvalid   = "INVALID"
	RowStatusCommitted = "COMMITTED"
	RowStatusFailed    = "FAILED"
	RowStatusSkipped   = "SKIPPED"
)

type OrderRow struct {
	RowReference         string         `json:"rowReference"`
	CustomerID           string         `json:"customerId"`
	CustomerReference    string         `json:"customerReference"`
	ServiceCatalogItemID string         `json:"serviceCatalogItemId"`
	ContractID           string         `json:"contractId"`
	Timezone             string         `json:"timezone"`
	Priority             string         `json:"priority"`
	SpecialInstructions  string         `json:"specialInstructions"`
	ServiceFields        map[string]any `json:"serviceFields"`
}

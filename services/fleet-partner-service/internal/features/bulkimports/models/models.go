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
	ID, OrganizationID, BranchID, MediaFileID, FileName, FileFormat, ContentType, Status string
	SizeBytes                                                                            int64
	TotalRows, ValidRows, InvalidRows, ImportedRows, FailedRows                          int
	ErrorReportMediaID, RequestFingerprint                                               string
	Version                                                                              int64
	CreatedBy                                                                            string
	CreatedAt, UpdatedAt                                                                 time.Time
	CompletedAt, CancelledAt                                                             *time.Time
	Attempts                                                                             int

	// Section 25C generic-engine fields. Zero-valued (empty string / 0) on
	// every legacy-engine row.
	ImportType        string
	Engine            string
	Mode              string
	SchemaVersion     int
	FileFingerprint   string
	ExternalReference string
	CommittedRows     int
	ValidationVersion int
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
	ID, OrganizationID, ImportID                                                            string
	RowNumber                                                                               int
	RowReference, RowFingerprint, Status, GeneratedOrderID, SafeErrorCode, SafeErrorMessage string
	Data                                                                                    OrderRow
	CreatedAt, UpdatedAt                                                                    time.Time

	// Section 25C generic-engine fields.
	ExternalReference         string
	SchemaVersion             int
	CommitStatus              string
	GeneratedSubjectReference string
	RowVersion                int
	ReplacesRowID             string
	// GenericData is the normalized row payload for a generic-engine row. The
	// legacy Data field stays typed to OrderRow and is never populated for a
	// generic row — the two engines' rows are read through separate
	// repository methods (ListRows for legacy, ListGenericRows for generic)
	// precisely so one is never accidentally unmarshalled as the other.
	GenericData map[string]any
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
	RowReference, CustomerID, CustomerReference, ServiceCatalogItemID, ContractID, Timezone, Priority, SpecialInstructions string
	ServiceFields                                                                                                          map[string]any
}

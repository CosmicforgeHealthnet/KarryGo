// Package models defines the Section 25D generic asynchronous export
// framework: a typed job that a registered dataset adapter fills in, with its
// output stored at Media exactly like every other file this service points
// at rather than owns.
package models

import "time"

// Job statuses. PENDING is the only status a caller may create; every other
// transition is worker- or download-driven.
const (
	StatusPending    = "PENDING"
	StatusRunning    = "RUNNING"
	StatusReady      = "READY"
	StatusFailed     = "FAILED"
	StatusExpired    = "EXPIRED"
	StatusCancelled  = "CANCELLED"
	StatusDeadLetter = "DEAD_LETTER"
)

// ExportType is the registered dataset an export job draws from. The set
// recognized by the database CHECK constraint is widened by a new migration
// each time an adapter is registered — the same pattern document categories
// and owner types already use in this service.
const (
	TypeOrders              = "ORDERS"
	TypeShipments           = "SHIPMENTS"
	TypeProvidersWorkforce  = "PROVIDERS_WORKFORCE"
	TypeAssetsMaintenance   = "ASSETS_MAINTENANCE"
	TypeWarehouseInventory  = "WAREHOUSE_INVENTORY"
	TypeComplianceIncidents = "COMPLIANCE_INCIDENTS"
	TypeInvoicesReceivables = "INVOICES_RECEIVABLES"
	TypeFulfilmentOrders    = "FULFILMENT_ORDERS"
	TypeSupportCases        = "SUPPORT_CASE_PROJECTIONS"
)

// validExportTypes is the Go-side mirror of the migration's CHECK constraint.
var validExportTypes = map[string]bool{
	TypeOrders: true, TypeShipments: true, TypeProvidersWorkforce: true,
	TypeAssetsMaintenance: true, TypeWarehouseInventory: true,
	TypeComplianceIncidents: true, TypeInvoicesReceivables: true,
	TypeFulfilmentOrders: true, TypeSupportCases: true,
}

// ValidExportType reports whether the type is one this service recognizes.
// A registered Exporter is a separate, additional requirement — recognizing
// the type does not by itself mean an adapter is wired to serve it.
func ValidExportType(exportType string) bool { return validExportTypes[exportType] }

// Formats. CSV is the only one implemented; the column is future-proofed for
// a second format without a schema change.
const FormatCSV = "CSV"

func ValidFormat(format string) bool { return format == FormatCSV }

// nonTerminalStatuses is what the worker's claim query looks for.
var nonTerminalStatuses = map[string]bool{StatusPending: true, StatusRunning: true}

// Terminal reports whether the job has reached a state no further worker
// action can move it out of.
func (j Job) Terminal() bool { return !nonTerminalStatuses[j.Status] }

// Job is one export request and its outcome.
type Job struct {
	ID                 string
	OrganizationID     string
	BranchID           string
	ExportType         string
	Format             string
	FilterSnapshot     JSONMap
	RequestedColumns   []string
	Status             string
	RequestFingerprint string
	IdempotencyKey     string
	SourceCutoffAt     *time.Time
	MediaFileID        string
	RowCount           int
	ExpiresAt          *time.Time
	CreatedBy          string
	Version            int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
	CompletedAt        *time.Time
	CancelledAt        *time.Time
	LeaseOwner         string
	LeaseExpiresAt     *time.Time
	Attempts           int
	LastErrorCode      string
}

// Downloadable reports whether a caller may currently obtain the file: ready,
// not expired, and no download is issued for a stale reference to a media
// object that expiry has already superseded.
func (j Job) Downloadable(now time.Time) bool {
	if j.Status != StatusReady {
		return false
	}
	if j.ExpiresAt != nil && !now.Before(*j.ExpiresAt) {
		return false
	}
	return j.MediaFileID != ""
}

// Attempt is one immutable worker outcome, mirroring bulk imports' attempt
// history.
type Attempt struct {
	ID             string
	OrganizationID string
	ExportID       string
	Attempt        int
	Outcome        string
	SafeErrorCode  string
	OccurredAt     time.Time
}

// JSONMap is a shallow string-keyed filter/metadata bag. It is cloned on
// read and write so a caller mutating the map it passed in cannot reach back
// into a stored job, and a stored job cannot be mutated through a view handed
// to a caller.
type JSONMap map[string]any

func (m JSONMap) Clone() JSONMap {
	if m == nil {
		return JSONMap{}
	}
	clone := make(JSONMap, len(m))
	for k, v := range m {
		clone[k] = v
	}
	return clone
}

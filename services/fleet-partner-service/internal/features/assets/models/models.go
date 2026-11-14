// Package models holds the Section 18 asset domain: organization assets,
// provider-to-asset assignments, asset documents, breakdowns, spare parts and
// stock movements, meter readings, defects, tyres, operating-cost history and
// the pure rules over them. Canonical vehicles, trips, Media bytes, provider
// identities and financial ledgers remain externally authoritative.
package models

import (
	"encoding/json"
	"regexp"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/money"
)

// Flags are the Section 18 capability switches. Assets, inspections,
// maintenance and fuel share one set so a single configuration drives the
// whole section.
type Flags struct{ Assets, Assignments, Inspections, Maintenance, Breakdowns, Inventory, Fuel, Workers, KillSwitch bool }

type Asset struct {
	ID                    string     `json:"id"`
	OrganizationID        string     `json:"organizationId"`
	BranchID              string     `json:"branchId"`
	AssetCode             string     `json:"assetCode"`
	AssetType             string     `json:"assetType"`
	PlatformVehicleID     string     `json:"platformVehicleId"`
	OwnershipType         string     `json:"ownershipType"`
	RegistrationReference string     `json:"registrationReference"`
	Manufacturer          string     `json:"manufacturer"`
	Model                 string     `json:"model"`
	CapacityUnit          string     `json:"capacityUnit"`
	OperationalStatus     string     `json:"operationalStatus"`
	LifecycleStatus       string     `json:"lifecycleStatus"`
	Year                  *int       `json:"year"`
	Capacity              float64    `json:"capacity"`
	Odometer              float64    `json:"odometer"`
	OperatingHours        float64    `json:"operatingHours"`
	Version               int64      `json:"version"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             time.Time  `json:"updatedAt"`
	ArchivedAt            *time.Time `json:"archivedAt"`
	RetiredAt             *time.Time `json:"retiredAt"`
}
type Assignment struct {
	ID                   string     `json:"id"`
	OrganizationID       string     `json:"organizationId"`
	AssetID              string     `json:"assetId"`
	ProviderMembershipID string     `json:"providerMembershipId"`
	ShiftID              string     `json:"shiftId"`
	AssignmentType       string     `json:"assignmentType"`
	Status               string     `json:"status"`
	StartAt              time.Time  `json:"startAt"`
	EndAt                *time.Time `json:"endAt"`
	Version              int64      `json:"version"`
	CreatedAt            time.Time  `json:"createdAt"`
	UpdatedAt            time.Time  `json:"updatedAt"`
}
type Document struct {
	ID                    string     `json:"id"`
	OrganizationID        string     `json:"organizationId"`
	AssetID               string     `json:"assetId"`
	MediaFileID           string     `json:"mediaFileId"`
	DocumentType          string     `json:"documentType"`
	VerificationReference string     `json:"verificationReference"`
	VerificationStatus    string     `json:"verificationStatus"`
	IssuedAt              *time.Time `json:"issuedAt"`
	ExpiresAt             *time.Time `json:"expiresAt"`
	Version               int64      `json:"version"`
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             time.Time  `json:"updatedAt"`
}
type Breakdown struct {
	ID                   string    `json:"id"`
	OrganizationID       string    `json:"organizationId"`
	AssetID              string    `json:"assetId"`
	ProviderMembershipID string    `json:"providerMembershipId"`
	TripReference        string    `json:"tripReference"`
	LocationReference    string    `json:"locationReference"`
	Severity             string    `json:"severity"`
	TripImpact           string    `json:"tripImpact"`
	RescueStatus         string    `json:"rescueStatus"`
	TowingReference      string    `json:"towingReference"`
	ReplacementAssetID   string    `json:"replacementAssetId"`
	Status               string    `json:"status"`
	OccurredAt           time.Time `json:"occurredAt"`
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
	Version              int64     `json:"version"`
}
type Part struct {
	ID                        string    `json:"id"`
	OrganizationID            string    `json:"organizationId"`
	BranchID                  string    `json:"branchId"`
	SKU                       string    `json:"sku"`
	Name                      string    `json:"name"`
	StockUnit                 string    `json:"stockUnit"`
	WarrantyInformation       string    `json:"warrantyInformation"`
	Status                    string    `json:"status"`
	CurrentQuantityProjection float64   `json:"currentQuantityProjection"`
	ReorderLevel              float64   `json:"reorderLevel"`
	Version                   int64     `json:"version"`
	CreatedAt                 time.Time `json:"createdAt"`
	UpdatedAt                 time.Time `json:"updatedAt"`
}
type StockMovement struct {
	ID                string    `json:"id"`
	OrganizationID    string    `json:"organizationId"`
	PartID            string    `json:"partId"`
	LocationReference string    `json:"locationReference"`
	MovementType      string    `json:"movementType"`
	Reason            string    `json:"reason"`
	WorkOrderID       string    `json:"workOrderId"`
	CorrelationID     string    `json:"correlationId"`
	CreatedBy         string    `json:"createdBy"`
	Quantity          float64   `json:"quantity"`
	CreatedAt         time.Time `json:"createdAt"`
}
type Eligibility struct {
	Eligible                      bool `json:"eligible"`
	HardBlocks, Warnings, Reasons []string
}

// Page selects one page of a list. Offset paging is kept for compatibility;
// After/AfterID is the keyset position of the last row of the previous page.
type Page struct {
	BranchID, Status string
	Limit, Offset    int
	After            *time.Time
	AfterID          string
}

// StockSign is +1 for stock-increasing movement types and -1 for issues and
// outbound transfers.
func StockSign(movementType string) float64 {
	if movementType == "ISSUE" || movementType == "TRANSFER_OUT" {
		return -1
	}
	return 1
}

// UnassignInput ends a provider-to-asset allocation.
type UnassignInput struct {
	AssignmentID    string `json:"assignmentId"`
	Reason          string `json:"reason"`
	ExpectedVersion int64  `json:"expectedVersion"`
}

// ---------------------------------------------------------------------------
// Meter readings
// ---------------------------------------------------------------------------

// Meter anomaly thresholds. An anomalous reading is still recorded (spec 48.2:
// review flags, never silent rejection of a valid emergency); it is flagged
// and raises a review alert.
const (
	maxOdometerRatePerHour  = 150.0  // distance units per elapsed hour
	maxOdometerSingleJump   = 5000.0 // distance units in one reading
	maxEngineHoursSlack     = 0.25   // engine hours beyond elapsed wall-clock hours
	maxEngineHoursSingleJmp = 500.0
)

type MeterReadingInput struct {
	AssetID         string    `json:"assetId"`
	MeterType       string    `json:"meterType"`
	Reading         float64   `json:"reading"`
	Source          string    `json:"source"`
	SourceReference string    `json:"sourceReference"`
	RecordedAt      time.Time `json:"recordedAt"`
}

type MeterReading struct {
	ID              string    `json:"id"`
	OrganizationID  string    `json:"organizationId"`
	AssetID         string    `json:"assetId"`
	MeterType       string    `json:"meterType"`
	Reading         float64   `json:"reading"`
	PreviousReading float64   `json:"previousReading"`
	Source          string    `json:"source"`
	SourceReference string    `json:"sourceReference,omitempty"`
	RecordedAt      time.Time `json:"recordedAt"`
	Anomaly         bool      `json:"anomaly"`
	AnomalyReasons  []string  `json:"anomalyReasons"`
	RecordedBy      string    `json:"recordedBy"`
	CreatedAt       time.Time `json:"createdAt"`
	DueScheduleIDs  []string  `json:"dueScheduleIds,omitempty"`
}

// MeterAnomalies is the pure anomaly rule set for one reading.
func MeterAnomalies(meterType string, previous, reading float64, previousAt *time.Time, at time.Time) []string {
	reasons := []string{}
	delta := reading - previous
	hours := -1.0
	if previousAt != nil {
		hours = at.Sub(*previousAt).Hours()
	}
	switch meterType {
	case "ODOMETER":
		if delta > maxOdometerSingleJump {
			reasons = append(reasons, "LARGE_SINGLE_JUMP")
		}
		if hours >= 0 && delta > 0 && delta > maxOdometerRatePerHour*hours {
			reasons = append(reasons, "IMPLAUSIBLE_DISTANCE_RATE")
		}
	case "ENGINE_HOURS":
		if delta > maxEngineHoursSingleJmp {
			reasons = append(reasons, "LARGE_SINGLE_JUMP")
		}
		if hours >= 0 && delta > hours+maxEngineHoursSlack {
			reasons = append(reasons, "ENGINE_HOURS_EXCEED_ELAPSED_TIME")
		}
	}
	return reasons
}

// ---------------------------------------------------------------------------
// Asset history and operating costs
// ---------------------------------------------------------------------------

type AssetHistoryEntry struct {
	ID                        string          `json:"id"`
	PreviousOperationalStatus string          `json:"previousOperationalStatus"`
	NewOperationalStatus      string          `json:"newOperationalStatus"`
	PreviousLifecycleStatus   string          `json:"previousLifecycleStatus"`
	NewLifecycleStatus        string          `json:"newLifecycleStatus"`
	Reason                    string          `json:"reason"`
	ActorID                   string          `json:"actorId"`
	Version                   int64           `json:"version"`
	Snapshot                  json.RawMessage `json:"snapshot"`
	OccurredAt                time.Time       `json:"occurredAt"`
}

// CurrencyPattern is an ISO 4217 currency code.
var CurrencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

type CostInput struct {
	CostType        string    `json:"costType"`
	SourceType      string    `json:"sourceType"`
	SourceReference string    `json:"sourceReference"`
	AmountMinor     int64     `json:"amountMinor"`
	Currency        string    `json:"currency"`
	IncurredAt      time.Time `json:"incurredAt"`
	Description     string    `json:"description"`
}

type CostEntry struct {
	ID              string    `json:"id"`
	OrganizationID  string    `json:"organizationId"`
	AssetID         string    `json:"assetId"`
	CostType        string    `json:"costType"`
	SourceType      string    `json:"sourceType"`
	SourceReference string    `json:"sourceReference,omitempty"`
	AmountMinor     int64     `json:"amountMinor"`
	Currency        string    `json:"currency"`
	IncurredAt      time.Time `json:"incurredAt"`
	Description     string    `json:"description,omitempty"`
	RecordedBy      string    `json:"recordedBy"`
	CreatedAt       time.Time `json:"createdAt"`
}

// Amount is the entry's amount as a Money value.
func (c CostEntry) Amount() money.Money {
	return money.Money{AmountMinor: c.AmountMinor, Currency: c.Currency}
}

type CostTotal struct {
	CostType    string `json:"costType"`
	Currency    string `json:"currency"`
	AmountMinor int64  `json:"amountMinor"`
}

// Amount is the total as a Money value.
func (c CostTotal) Amount() money.Money {
	return money.Money{AmountMinor: c.AmountMinor, Currency: c.Currency}
}

type CostHistory struct {
	AssetID string      `json:"assetId"`
	Entries []CostEntry `json:"entries"`
	Totals  []CostTotal `json:"totals"`
}

// CostTypes and CostSourceTypes are the accepted operating-cost categories.
var (
	CostTypes       = map[string]bool{"MAINTENANCE": true, "FUEL": true, "PARTS": true, "TYRES": true, "OTHER": true}
	CostSourceTypes = map[string]bool{"WORK_ORDER": true, "FUEL_RECORD": true, "STOCK_MOVEMENT": true, "TYRE": true, "MANUAL": true}
)

// ---------------------------------------------------------------------------
// Defects
// ---------------------------------------------------------------------------

// Asset defects (spec 48.2, 48.3). Lifecycle OPEN -> DEFERRED|REPAIRING ->
// RESOLVED. A CRITICAL defect blocks the asset in the same transaction, can
// never be deferred, can only be resolved once its repair work order is
// complete, and never unblocks the asset by itself: only an authorized
// return-to-service decision does.
const (
	DefectOpen      = "OPEN"
	DefectDeferred  = "DEFERRED"
	DefectRepairing = "REPAIRING"
	DefectResolved  = "RESOLVED"
)

type Defect struct {
	ID              string     `json:"id"`
	OrganizationID  string     `json:"organizationId"`
	BranchID        string     `json:"branchId"`
	AssetID         string     `json:"assetId"`
	SourceType      string     `json:"sourceType"`
	SourceReference string     `json:"sourceReference,omitempty"`
	ItemCode        string     `json:"itemCode,omitempty"`
	Description     string     `json:"description"`
	Severity        string     `json:"severity"`
	Critical        bool       `json:"critical"`
	Status          string     `json:"status"`
	WorkOrderID     string     `json:"workOrderId,omitempty"`
	DeferredUntil   *time.Time `json:"deferredUntil,omitempty"`
	DeferralReason  string     `json:"deferralReason,omitempty"`
	ResolutionNotes string     `json:"resolutionNotes,omitempty"`
	ResolvedBy      string     `json:"resolvedBy,omitempty"`
	ResolvedAt      *time.Time `json:"resolvedAt,omitempty"`
	ReportedBy      string     `json:"reportedBy"`
	Version         int64      `json:"version"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

type ReportDefectInput struct {
	AssetID         string `json:"assetId"`
	SourceType      string `json:"sourceType"`
	SourceReference string `json:"sourceReference"`
	ItemCode        string `json:"itemCode"`
	Description     string `json:"description"`
	Severity        string `json:"severity"`
}

type DefectAction struct {
	ExpectedVersion int64      `json:"expectedVersion"`
	Reason          string     `json:"reason"`
	Notes           string     `json:"notes"`
	DeferredUntil   *time.Time `json:"deferredUntil"`
	WorkOrderID     string     `json:"workOrderId"`
}

type DefectFilter struct{ AssetID, Status, Severity string }

// DefectSeverities is the closed severity scale.
var DefectSeverities = map[string]bool{"LOW": true, "MEDIUM": true, "HIGH": true, "CRITICAL": true}

// ---------------------------------------------------------------------------
// Tyres
// ---------------------------------------------------------------------------

type Tyre struct {
	ID               string     `json:"id"`
	OrganizationID   string     `json:"organizationId"`
	BranchID         string     `json:"branchId"`
	SerialNumber     string     `json:"serialNumber"`
	Brand            string     `json:"brand,omitempty"`
	Size             string     `json:"size,omitempty"`
	Status           string     `json:"status"`
	AssetID          string     `json:"assetId,omitempty"`
	Position         string     `json:"position,omitempty"`
	InstalledAt      *time.Time `json:"installedAt,omitempty"`
	LastTreadDepthMM *float64   `json:"lastTreadDepthMm,omitempty"`
	MinTreadDepthMM  float64    `json:"minTreadDepthMm"`
	Version          int64      `json:"version"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	// LowTreadDefectID is set on a tread reading that opened a defect.
	LowTreadDefectID string `json:"lowTreadDefectId,omitempty"`
}

type RegisterTyreInput struct {
	BranchID        string   `json:"branchId"`
	SerialNumber    string   `json:"serialNumber"`
	Brand           string   `json:"brand"`
	Size            string   `json:"size"`
	MinTreadDepthMM *float64 `json:"minTreadDepthMm"`
	TreadDepthMM    *float64 `json:"treadDepthMm"`
}

type TyreAction struct {
	ExpectedVersion int64    `json:"expectedVersion"`
	AssetID         string   `json:"assetId"`
	Position        string   `json:"position"`
	ToPosition      string   `json:"toPosition"`
	Reason          string   `json:"reason"`
	Scrap           bool     `json:"scrap"`
	TreadDepthMM    *float64 `json:"treadDepthMm"`
	Odometer        *float64 `json:"odometer"`
}

// AssetLock is the locked safety state of one asset.
type AssetLock struct {
	BranchID, OperationalStatus, LifecycleStatus string
	Version                                      int64
	Odometer, OperatingHours                     float64
}

// ResolvedDefect is a defect resolved by a return-to-service decision.
type ResolvedDefect struct {
	ID      string
	Version int64
}

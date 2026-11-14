package assets

import (
	"encoding/json"
	"time"
)

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
type Template struct {
	ID               string          `json:"id"`
	OrganizationID   string          `json:"organizationId"`
	Name             string          `json:"name"`
	Status           string          `json:"status"`
	AssetTypes       []string        `json:"assetTypes"`
	ServiceTypes     []string        `json:"serviceTypes"`
	ChecklistItems   json.RawMessage `json:"checklistItems"`
	CriticalityRules json.RawMessage `json:"criticalityRules"`
	Version          int64           `json:"version"`
	CreatedAt        time.Time       `json:"createdAt"`
	UpdatedAt        time.Time       `json:"updatedAt"`
}
type Inspection struct {
	ID                    string          `json:"id"`
	OrganizationID        string          `json:"organizationId"`
	AssetID               string          `json:"assetId"`
	TemplateID            string          `json:"templateId"`
	InspectorMembershipID string          `json:"inspectorMembershipId"`
	AssignmentID          string          `json:"assignmentId"`
	TripReference         string          `json:"tripReference"`
	Status                string          `json:"status"`
	OverallResult         string          `json:"overallResult"`
	DefectSummary         json.RawMessage `json:"defectSummary"`
	EvidenceMediaIDs      json.RawMessage `json:"evidenceMediaIds"`
	StartedAt             *time.Time      `json:"startedAt"`
	CompletedAt           *time.Time      `json:"completedAt"`
	Version               int64           `json:"version"`
	CreatedAt             time.Time       `json:"createdAt"`
	UpdatedAt             time.Time       `json:"updatedAt"`
}
type Answer struct {
	ItemCode         string   `json:"itemCode"`
	Passed           bool     `json:"passed"`
	Advisory         bool     `json:"advisory"`
	EvidenceMediaIDs []string `json:"evidenceMediaIds"`
}
type Schedule struct {
	ID              string     `json:"id"`
	OrganizationID  string     `json:"organizationId"`
	AssetID         string     `json:"assetId"`
	TriggerType     string     `json:"triggerType"`
	Status          string     `json:"status"`
	IntervalValue   *float64   `json:"intervalValue"`
	NextDueOdometer *float64   `json:"nextDueOdometer"`
	NextDueHours    *float64   `json:"nextDueHours"`
	NextDueDate     *time.Time `json:"nextDueDate"`
	Critical        bool       `json:"critical"`
	Version         int64      `json:"version"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}
type WorkOrder struct {
	ID                    string          `json:"id"`
	OrganizationID        string          `json:"organizationId"`
	AssetID               string          `json:"assetId"`
	SourceType            string          `json:"sourceType"`
	SourceReference       string          `json:"sourceReference"`
	Priority              string          `json:"priority"`
	Diagnosis             string          `json:"diagnosis"`
	RequiredWork          string          `json:"requiredWork"`
	Status                string          `json:"status"`
	ApprovedBy            string          `json:"approvedBy"`
	ReturnToServiceStatus string          `json:"returnToServiceStatus"`
	ApprovedAt            *time.Time      `json:"approvedAt"`
	StartedAt             *time.Time      `json:"startedAt"`
	CompletedAt           *time.Time      `json:"completedAt"`
	DowntimeStart         *time.Time      `json:"downtimeStart"`
	DowntimeEnd           *time.Time      `json:"downtimeEnd"`
	EvidenceMediaIDs      json.RawMessage `json:"evidenceMediaIds"`
	Version               int64           `json:"version"`
	CreatedAt             time.Time       `json:"createdAt"`
	UpdatedAt             time.Time       `json:"updatedAt"`
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
type FuelRecord struct {
	ID                   string    `json:"id"`
	OrganizationID       string    `json:"organizationId"`
	AssetID              string    `json:"assetId"`
	ProviderMembershipID string    `json:"providerMembershipId"`
	FuelOrEnergyType     string    `json:"fuelOrEnergyType"`
	Unit                 string    `json:"unit"`
	ReceiptMediaID       string    `json:"receiptMediaId"`
	SupplierReference    string    `json:"supplierReference"`
	ApprovalStatus       string    `json:"approvalStatus"`
	ApprovedBy           string    `json:"approvedBy"`
	Quantity             float64   `json:"quantity"`
	Odometer             *float64  `json:"odometer"`
	OperatingHours       *float64  `json:"operatingHours"`
	RecordedAt           time.Time `json:"recordedAt"`
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
	Version              int64     `json:"version"`
}
type Eligibility struct {
	Eligible                      bool `json:"eligible"`
	HardBlocks, Warnings, Reasons []string
}
type Page struct {
	BranchID, Status string
	Limit, Offset    int
}

func ScheduleDue(v Schedule, a Asset, now time.Time) bool {
	switch v.TriggerType {
	case "DATE":
		return v.NextDueDate != nil && !v.NextDueDate.After(now)
	case "MILEAGE":
		return v.NextDueOdometer != nil && a.Odometer >= *v.NextDueOdometer
	case "OPERATING_HOURS":
		return v.NextDueHours != nil && a.OperatingHours >= *v.NextDueHours
	case "COMBINED":
		return (v.NextDueDate != nil && !v.NextDueDate.After(now)) || (v.NextDueOdometer != nil && a.Odometer >= *v.NextDueOdometer) || (v.NextDueHours != nil && a.OperatingHours >= *v.NextDueHours)
	}
	return false
}

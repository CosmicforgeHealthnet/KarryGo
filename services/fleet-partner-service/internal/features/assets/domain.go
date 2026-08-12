package assets

import (
	"encoding/json"
	"time"
)

type Flags struct{ Assets, Assignments, Inspections, Maintenance, Breakdowns, Inventory, Fuel, Workers, KillSwitch bool }
type Asset struct {
	ID, OrganizationID, BranchID, AssetCode, AssetType, PlatformVehicleID, OwnershipType, RegistrationReference, Manufacturer, Model, CapacityUnit, OperationalStatus, LifecycleStatus string
	Year                                                                                                                                                                               *int
	Capacity, Odometer, OperatingHours                                                                                                                                                 float64
	Version                                                                                                                                                                            int64
	CreatedAt, UpdatedAt                                                                                                                                                               time.Time
	ArchivedAt, RetiredAt                                                                                                                                                              *time.Time
}
type Assignment struct {
	ID, OrganizationID, AssetID, ProviderMembershipID, ShiftID, AssignmentType, Status string
	StartAt                                                                            time.Time
	EndAt                                                                              *time.Time
	Version                                                                            int64
	CreatedAt, UpdatedAt                                                               time.Time
}
type Document struct {
	ID, OrganizationID, AssetID, MediaFileID, DocumentType, VerificationReference, VerificationStatus string
	IssuedAt, ExpiresAt                                                                               *time.Time
	Version                                                                                           int64
	CreatedAt, UpdatedAt                                                                              time.Time
}
type Template struct {
	ID, OrganizationID, Name, Status string
	AssetTypes, ServiceTypes         []string
	ChecklistItems, CriticalityRules json.RawMessage
	Version                          int64
	CreatedAt, UpdatedAt             time.Time
}
type Inspection struct {
	ID, OrganizationID, AssetID, TemplateID, InspectorMembershipID, AssignmentID, TripReference, Status, OverallResult string
	DefectSummary, EvidenceMediaIDs                                                                                    json.RawMessage
	StartedAt, CompletedAt                                                                                             *time.Time
	Version                                                                                                            int64
	CreatedAt, UpdatedAt                                                                                               time.Time
}
type Answer struct {
	ItemCode         string   `json:"itemCode"`
	Passed           bool     `json:"passed"`
	Advisory         bool     `json:"advisory"`
	EvidenceMediaIDs []string `json:"evidenceMediaIds"`
}
type Schedule struct {
	ID, OrganizationID, AssetID, TriggerType, Status string
	IntervalValue, NextDueOdometer, NextDueHours     *float64
	NextDueDate                                      *time.Time
	Critical                                         bool
	Version                                          int64
	CreatedAt, UpdatedAt                             time.Time
}
type WorkOrder struct {
	ID, OrganizationID, AssetID, SourceType, SourceReference, Priority, Diagnosis, RequiredWork, Status, ApprovedBy, ReturnToServiceStatus string
	ApprovedAt, StartedAt, CompletedAt, DowntimeStart, DowntimeEnd                                                                         *time.Time
	EvidenceMediaIDs                                                                                                                       json.RawMessage
	Version                                                                                                                                int64
	CreatedAt, UpdatedAt                                                                                                                   time.Time
}
type Breakdown struct {
	ID, OrganizationID, AssetID, ProviderMembershipID, TripReference, LocationReference, Severity, TripImpact, RescueStatus, TowingReference, ReplacementAssetID, Status string
	OccurredAt, CreatedAt, UpdatedAt                                                                                                                                     time.Time
	Version                                                                                                                                                              int64
}
type Part struct {
	ID, OrganizationID, BranchID, SKU, Name, StockUnit, WarrantyInformation, Status string
	CurrentQuantityProjection, ReorderLevel                                         float64
	Version                                                                         int64
	CreatedAt, UpdatedAt                                                            time.Time
}
type StockMovement struct {
	ID, OrganizationID, PartID, LocationReference, MovementType, Reason, WorkOrderID, CorrelationID, CreatedBy string
	Quantity                                                                                                   float64
	CreatedAt                                                                                                  time.Time
}
type FuelRecord struct {
	ID, OrganizationID, AssetID, ProviderMembershipID, FuelOrEnergyType, Unit, ReceiptMediaID, SupplierReference, ApprovalStatus, ApprovedBy string
	Quantity                                                                                                                                 float64
	Odometer, OperatingHours                                                                                                                 *float64
	RecordedAt, CreatedAt, UpdatedAt                                                                                                         time.Time
	Version                                                                                                                                  int64
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

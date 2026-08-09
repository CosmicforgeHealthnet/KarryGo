package warehouse

import (
	"encoding/json"
	"time"
)

type Flags struct{ Warehouses, Inventory, Fulfilment, Dock, Yard, Terminal, Returns, Workers, KillSwitch bool }
type Warehouse struct {
	ID, OrganizationID, BranchID, Code, Name, SiteReference, AddressReference, Timezone string
	OperatingHours                                                                      json.RawMessage
	Status                                                                              string
	Version                                                                             int64
	CreatedAt, UpdatedAt                                                                time.Time
	ArchivedAt                                                                          *time.Time
}
type Location struct {
	ID, OrganizationID, WarehouseID, ParentLocationID, Code, Name, LocationType string
	Capacity                                                                    *float64
	CapacityUnit                                                                string
	Restrictions                                                                json.RawMessage
	Status                                                                      string
	Version                                                                     int64
	CreatedAt, UpdatedAt                                                        time.Time
}
type Item struct {
	ID, OrganizationID, WarehouseID, LocationID, OwnerType, OwnerReference, SKU, BatchReference, LotReference, SerialReference string
	QuantityProjection, ReservedQuantity                                                                                       float64
	Unit, Status                                                                                                               string
	ExpiryAt                                                                                                                   *time.Time
	Condition                                                                                                                  string
	Version                                                                                                                    int64
	CreatedAt, UpdatedAt                                                                                                       time.Time
}
type Movement struct {
	ID, OrganizationID, InventoryItemID, MovementType, FromLocationID, ToLocationID            string
	Quantity                                                                                   float64
	ReasonCode, ActorReference, FulfilmentOrderID, TerminalTransferID, ReturnID, CorrelationID string
	ProjectionBefore, ProjectionAfter                                                          float64
	CreatedAt                                                                                  time.Time
}
type ReceiptInput struct {
	WarehouseID, LocationID, OwnerType, OwnerReference, SKU, BatchReference, LotReference, SerialReference, Unit, Condition, PackageID, ShipmentID, FromActorType, FromActorReference, EvidenceMediaID string
	Quantity                                                                                                                                                                                           float64
	ExpiryAt                                                                                                                                                                                           *time.Time
	CorrelationID                                                                                                                                                                                      string
}
type MovementInput struct {
	InventoryItemID, MovementType, ToLocationID, ReasonCode, FulfilmentOrderID, TerminalTransferID, ReturnID, CorrelationID string
	Quantity                                                                                                                float64
	Increase                                                                                                                bool
	ExpectedVersion                                                                                                         int64
}
type FulfilmentOrder struct {
	ID, OrganizationID, WarehouseID, CustomerReference, SourceOrderID, Priority, Status string
	RequestedDispatchAt                                                                 *time.Time
	AssignedTeamReference                                                               string
	PackingRequirements                                                                 json.RawMessage
	ProofPolicyReference, ShipmentReference, ManifestReference                          string
	Version                                                                             int64
	CreatedAt, UpdatedAt                                                                time.Time
	CompletedAt                                                                         *time.Time
	Lines                                                                               []FulfilmentLine
}
type FulfilmentLine struct {
	ID, OrganizationID, FulfilmentOrderID, InventoryItemID string
	RequestedQuantity, PickedQuantity                      float64
	PackageReference                                       string
	Version                                                int64
	CreatedAt, UpdatedAt                                   time.Time
}
type DockAppointment struct {
	ID, OrganizationID, WarehouseID, DockLocationID, CarrierReference, ProviderReference, VehicleOrTruckReference, AppointmentType string
	WindowStart, WindowEnd                                                                                                         time.Time
	Status                                                                                                                         string
	CheckedInAt, LoadingStartedAt, LoadingCompletedAt, CheckedOutAt                                                                *time.Time
	Version                                                                                                                        int64
	CreatedAt, UpdatedAt                                                                                                           time.Time
}
type YardVisit struct {
	ID, OrganizationID, WarehouseID, ProviderReference, VehicleOrTruckReference, GateReference, BayReference, SealReference, SecurityStatus, Status string
	CheckedInAt, EnteredYardAt, ExitedYardAt, CheckedOutAt                                                                                          *time.Time
	Version                                                                                                                                         int64
	CreatedAt, UpdatedAt                                                                                                                            time.Time
}
type TerminalTransfer struct {
	ID, OrganizationID, OriginWarehouseID, DestinationWarehouseID, ManifestID, RouteReference, Status string
	DepartedAt, ArrivedAt                                                                             *time.Time
	OriginSnapshotReference, DestinationSnapshotReference, ReconciliationStatus                       string
	Version                                                                                           int64
	CreatedAt, UpdatedAt                                                                              time.Time
}
type Return struct {
	ID, OrganizationID, WarehouseID, SourceOrderID, SourceShipmentID, PackageID, ReasonCode, Condition, Disposition, FinancialReference, Status, InventoryItemID string
	Version                                                                                                                                                      int64
	CreatedAt, UpdatedAt                                                                                                                                         time.Time
	CompletedAt                                                                                                                                                  *time.Time
}

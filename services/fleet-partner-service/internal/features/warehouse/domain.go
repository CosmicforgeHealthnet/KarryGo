package warehouse

import (
	"encoding/json"
	"time"
)

type Flags struct{ Warehouses, Inventory, Fulfilment, Dock, Yard, Terminal, Returns, Workers, KillSwitch bool }
type Warehouse struct {
	ID               string          `json:"id"`
	OrganizationID   string          `json:"organizationId"`
	BranchID         string          `json:"branchId"`
	Code             string          `json:"code"`
	Name             string          `json:"name"`
	SiteReference    string          `json:"siteReference"`
	AddressReference string          `json:"addressReference"`
	Timezone         string          `json:"timezone"`
	OperatingHours   json.RawMessage `json:"operatingHours"`
	Status           string          `json:"status"`
	Version          int64           `json:"version"`
	CreatedAt        time.Time       `json:"createdAt"`
	UpdatedAt        time.Time       `json:"updatedAt"`
	ArchivedAt       *time.Time      `json:"archivedAt"`
}
type Location struct {
	ID               string          `json:"id"`
	OrganizationID   string          `json:"organizationId"`
	WarehouseID      string          `json:"warehouseId"`
	ParentLocationID string          `json:"parentLocationId"`
	Code             string          `json:"code"`
	Name             string          `json:"name"`
	LocationType     string          `json:"locationType"`
	Capacity         *float64        `json:"capacity"`
	CapacityUnit     string          `json:"capacityUnit"`
	Restrictions     json.RawMessage `json:"restrictions"`
	Status           string          `json:"status"`
	Version          int64           `json:"version"`
	CreatedAt        time.Time       `json:"createdAt"`
	UpdatedAt        time.Time       `json:"updatedAt"`
}
type Item struct {
	ID                 string     `json:"id"`
	OrganizationID     string     `json:"organizationId"`
	WarehouseID        string     `json:"warehouseId"`
	LocationID         string     `json:"locationId"`
	OwnerType          string     `json:"ownerType"`
	OwnerReference     string     `json:"ownerReference"`
	SKU                string     `json:"sku"`
	BatchReference     string     `json:"batchReference"`
	LotReference       string     `json:"lotReference"`
	SerialReference    string     `json:"serialReference"`
	QuantityProjection float64    `json:"quantityProjection"`
	ReservedQuantity   float64    `json:"reservedQuantity"`
	Unit               string     `json:"unit"`
	Status             string     `json:"status"`
	ExpiryAt           *time.Time `json:"expiryAt"`
	Condition          string     `json:"condition"`
	Version            int64      `json:"version"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}
type Movement struct {
	ID                 string    `json:"id"`
	OrganizationID     string    `json:"organizationId"`
	InventoryItemID    string    `json:"inventoryItemId"`
	MovementType       string    `json:"movementType"`
	FromLocationID     string    `json:"fromLocationId"`
	ToLocationID       string    `json:"toLocationId"`
	Quantity           float64   `json:"quantity"`
	ReasonCode         string    `json:"reasonCode"`
	ActorReference     string    `json:"actorReference"`
	FulfilmentOrderID  string    `json:"fulfilmentOrderId"`
	TerminalTransferID string    `json:"terminalTransferId"`
	ReturnID           string    `json:"returnId"`
	CorrelationID      string    `json:"correlationId"`
	ProjectionBefore   float64   `json:"projectionBefore"`
	ProjectionAfter    float64   `json:"projectionAfter"`
	CreatedAt          time.Time `json:"createdAt"`
}
type ReceiptInput struct {
	WarehouseID        string     `json:"warehouseId"`
	LocationID         string     `json:"locationId"`
	OwnerType          string     `json:"ownerType"`
	OwnerReference     string     `json:"ownerReference"`
	SKU                string     `json:"sku"`
	BatchReference     string     `json:"batchReference"`
	LotReference       string     `json:"lotReference"`
	SerialReference    string     `json:"serialReference"`
	Unit               string     `json:"unit"`
	Condition          string     `json:"condition"`
	PackageID          string     `json:"packageId"`
	ShipmentID         string     `json:"shipmentId"`
	FromActorType      string     `json:"fromActorType"`
	FromActorReference string     `json:"fromActorReference"`
	EvidenceMediaID    string     `json:"evidenceMediaId"`
	Quantity           float64    `json:"quantity"`
	ExpiryAt           *time.Time `json:"expiryAt"`
	CorrelationID      string     `json:"correlationId"`
}
type MovementInput struct {
	InventoryItemID    string  `json:"inventoryItemId"`
	MovementType       string  `json:"movementType"`
	ToLocationID       string  `json:"toLocationId"`
	ReasonCode         string  `json:"reasonCode"`
	FulfilmentOrderID  string  `json:"fulfilmentOrderId"`
	TerminalTransferID string  `json:"terminalTransferId"`
	ReturnID           string  `json:"returnId"`
	CorrelationID      string  `json:"correlationId"`
	Quantity           float64 `json:"quantity"`
	Increase           bool    `json:"increase"`
	ExpectedVersion    int64   `json:"expectedVersion"`
}
type FulfilmentOrder struct {
	ID                    string           `json:"id"`
	OrganizationID        string           `json:"organizationId"`
	WarehouseID           string           `json:"warehouseId"`
	CustomerReference     string           `json:"customerReference"`
	SourceOrderID         string           `json:"sourceOrderId"`
	Priority              string           `json:"priority"`
	Status                string           `json:"status"`
	RequestedDispatchAt   *time.Time       `json:"requestedDispatchAt"`
	AssignedTeamReference string           `json:"assignedTeamReference"`
	PackingRequirements   json.RawMessage  `json:"packingRequirements"`
	ProofPolicyReference  string           `json:"proofPolicyReference"`
	ShipmentReference     string           `json:"shipmentReference"`
	ManifestReference     string           `json:"manifestReference"`
	Version               int64            `json:"version"`
	CreatedAt             time.Time        `json:"createdAt"`
	UpdatedAt             time.Time        `json:"updatedAt"`
	CompletedAt           *time.Time       `json:"completedAt"`
	Lines                 []FulfilmentLine `json:"lines"`
}
type FulfilmentLine struct {
	ID                string    `json:"id"`
	OrganizationID    string    `json:"organizationId"`
	FulfilmentOrderID string    `json:"fulfilmentOrderId"`
	InventoryItemID   string    `json:"inventoryItemId"`
	RequestedQuantity float64   `json:"requestedQuantity"`
	PickedQuantity    float64   `json:"pickedQuantity"`
	PackageReference  string    `json:"packageReference"`
	Version           int64     `json:"version"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}
type DockAppointment struct {
	ID                      string     `json:"id"`
	OrganizationID          string     `json:"organizationId"`
	WarehouseID             string     `json:"warehouseId"`
	DockLocationID          string     `json:"dockLocationId"`
	CarrierReference        string     `json:"carrierReference"`
	ProviderReference       string     `json:"providerReference"`
	VehicleOrTruckReference string     `json:"vehicleOrTruckReference"`
	AppointmentType         string     `json:"appointmentType"`
	WindowStart             time.Time  `json:"windowStart"`
	WindowEnd               time.Time  `json:"windowEnd"`
	Status                  string     `json:"status"`
	CheckedInAt             *time.Time `json:"checkedInAt"`
	LoadingStartedAt        *time.Time `json:"loadingStartedAt"`
	LoadingCompletedAt      *time.Time `json:"loadingCompletedAt"`
	CheckedOutAt            *time.Time `json:"checkedOutAt"`
	Version                 int64      `json:"version"`
	CreatedAt               time.Time  `json:"createdAt"`
	UpdatedAt               time.Time  `json:"updatedAt"`
}
type YardVisit struct {
	ID                      string     `json:"id"`
	OrganizationID          string     `json:"organizationId"`
	WarehouseID             string     `json:"warehouseId"`
	ProviderReference       string     `json:"providerReference"`
	VehicleOrTruckReference string     `json:"vehicleOrTruckReference"`
	GateReference           string     `json:"gateReference"`
	BayReference            string     `json:"bayReference"`
	SealReference           string     `json:"sealReference"`
	SecurityStatus          string     `json:"securityStatus"`
	Status                  string     `json:"status"`
	CheckedInAt             *time.Time `json:"checkedInAt"`
	EnteredYardAt           *time.Time `json:"enteredYardAt"`
	ExitedYardAt            *time.Time `json:"exitedYardAt"`
	CheckedOutAt            *time.Time `json:"checkedOutAt"`
	Version                 int64      `json:"version"`
	CreatedAt               time.Time  `json:"createdAt"`
	UpdatedAt               time.Time  `json:"updatedAt"`
}
type TerminalTransfer struct {
	ID                           string     `json:"id"`
	OrganizationID               string     `json:"organizationId"`
	OriginWarehouseID            string     `json:"originWarehouseId"`
	DestinationWarehouseID       string     `json:"destinationWarehouseId"`
	ManifestID                   string     `json:"manifestId"`
	RouteReference               string     `json:"routeReference"`
	Status                       string     `json:"status"`
	DepartedAt                   *time.Time `json:"departedAt"`
	ArrivedAt                    *time.Time `json:"arrivedAt"`
	OriginSnapshotReference      string     `json:"originSnapshotReference"`
	DestinationSnapshotReference string     `json:"destinationSnapshotReference"`
	ReconciliationStatus         string     `json:"reconciliationStatus"`
	Version                      int64      `json:"version"`
	CreatedAt                    time.Time  `json:"createdAt"`
	UpdatedAt                    time.Time  `json:"updatedAt"`
}
type Return struct {
	ID                 string     `json:"id"`
	OrganizationID     string     `json:"organizationId"`
	WarehouseID        string     `json:"warehouseId"`
	SourceOrderID      string     `json:"sourceOrderId"`
	SourceShipmentID   string     `json:"sourceShipmentId"`
	PackageID          string     `json:"packageId"`
	ReasonCode         string     `json:"reasonCode"`
	Condition          string     `json:"condition"`
	Disposition        string     `json:"disposition"`
	FinancialReference string     `json:"financialReference"`
	Status             string     `json:"status"`
	InventoryItemID    string     `json:"inventoryItemId"`
	Version            int64      `json:"version"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
	CompletedAt        *time.Time `json:"completedAt"`
}

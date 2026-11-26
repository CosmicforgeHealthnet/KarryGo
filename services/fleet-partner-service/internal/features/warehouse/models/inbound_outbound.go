package models

import (
	"encoding/json"
	"time"
)

// Section 51 inbound and outbound warehouse operations. Quantities are stock
// counts (not money), stored as numeric(18,3) like the rest of the stock
// model. Every stock change these operations cause is written as an
// immutable inventory movement.

// WarehouseReceipt is one expected inbound delivery with its lines.
type WarehouseReceipt struct {
	ID                   string        `json:"id"`
	OrganizationID       string        `json:"organizationId"`
	WarehouseID          string        `json:"warehouseId"`
	ReceiptNumber        string        `json:"receiptNumber"`
	SourceType           string        `json:"sourceType"`
	SourceReference      string        `json:"sourceReference"`
	ReceivingLocationID  string        `json:"receivingLocationId"`
	QuarantineLocationID string        `json:"quarantineLocationId"`
	YardVisitID          string        `json:"yardVisitId"`
	Status               string        `json:"status"`
	ExpectedAt           *time.Time    `json:"expectedAt"`
	ArrivedAt            *time.Time    `json:"arrivedAt"`
	ReceivedAt           *time.Time    `json:"receivedAt"`
	CompletedAt          *time.Time    `json:"completedAt"`
	Version              int64         `json:"version"`
	CreatedAt            time.Time     `json:"createdAt"`
	UpdatedAt            time.Time     `json:"updatedAt"`
	Lines                []ReceiptLine `json:"lines"`
}

// ReceiptLine is one expected SKU on a receipt and what was actually counted.
type ReceiptLine struct {
	ID                    string     `json:"id"`
	ReceiptID             string     `json:"receiptId"`
	LineNumber            int        `json:"lineNumber"`
	SKU                   string     `json:"sku"`
	OwnerType             string     `json:"ownerType"`
	OwnerReference        string     `json:"ownerReference"`
	LotReference          string     `json:"lotReference"`
	SerialReference       string     `json:"serialReference"`
	Unit                  string     `json:"unit"`
	ExpectedQuantity      float64    `json:"expectedQuantity"`
	ReceivedQuantity      float64    `json:"receivedQuantity"`
	DamagedQuantity       float64    `json:"damagedQuantity"`
	QuarantinedQuantity   float64    `json:"quarantinedQuantity"`
	AcceptedQuantity      float64    `json:"acceptedQuantity"`
	InventoryItemID       string     `json:"inventoryItemId"`
	QuarantineItemID      string     `json:"quarantineItemId"`
	PutawayLocationID     string     `json:"putawayLocationId"`
	Status                string     `json:"status"`
	DiscrepancyStatus     string     `json:"discrepancyStatus"`
	DiscrepancyResolution string     `json:"discrepancyResolution"`
	DiscrepancyNote       string     `json:"discrepancyNote"`
	ReceivedAt            *time.Time `json:"receivedAt"`
	Version               int64      `json:"version"`
}

// ReceiptCreateInput creates an expected receipt.
type ReceiptCreateInput struct {
	WarehouseID          string                   `json:"warehouseId"`
	ReceiptNumber        string                   `json:"receiptNumber"`
	SourceType           string                   `json:"sourceType"`
	SourceReference      string                   `json:"sourceReference"`
	ReceivingLocationID  string                   `json:"receivingLocationId"`
	QuarantineLocationID string                   `json:"quarantineLocationId"`
	ExpectedAt           *time.Time               `json:"expectedAt"`
	Lines                []ReceiptLineCreateInput `json:"lines"`
}

// ReceiptLineCreateInput is one expected line.
type ReceiptLineCreateInput struct {
	SKU               string  `json:"sku"`
	OwnerType         string  `json:"ownerType"`
	OwnerReference    string  `json:"ownerReference"`
	LotReference      string  `json:"lotReference"`
	SerialReference   string  `json:"serialReference"`
	Unit              string  `json:"unit"`
	ExpectedQuantity  float64 `json:"expectedQuantity"`
	PutawayLocationID string  `json:"putawayLocationId"`
}

// ReceiptCommandInput drives arrive, receive, resolve, complete and cancel.
type ReceiptCommandInput struct {
	ExpectedVersion int64              `json:"expectedVersion"`
	YardVisitID     string             `json:"yardVisitId"`
	Lines           []ReceiveLineInput `json:"lines"`
	LineID          string             `json:"lineId"`
	Resolution      string             `json:"resolution"`
	Note            string             `json:"note"`
	Reason          string             `json:"reason"`
}

// ReceiveLineInput is the physical count of one line. ScannedSKU must equal
// the line SKU: a wrong scan is refused rather than recorded.
type ReceiveLineInput struct {
	LineID              string  `json:"lineId"`
	ScannedSKU          string  `json:"scannedSku"`
	ReceivedQuantity    float64 `json:"receivedQuantity"`
	DamagedQuantity     float64 `json:"damagedQuantity"`
	QuarantinedQuantity float64 `json:"quarantinedQuantity"`
}

// PutawayTask moves accepted stock from the receiving area into storage.
type PutawayTask struct {
	ID                  string     `json:"id"`
	OrganizationID      string     `json:"organizationId"`
	WarehouseID         string     `json:"warehouseId"`
	ReceiptID           string     `json:"receiptId"`
	ReceiptLineID       string     `json:"receiptLineId"`
	InventoryItemID     string     `json:"inventoryItemId"`
	FromLocationID      string     `json:"fromLocationId"`
	SuggestedLocationID string     `json:"suggestedLocationId"`
	ToLocationID        string     `json:"toLocationId"`
	DestinationItemID   string     `json:"destinationItemId"`
	Quantity            float64    `json:"quantity"`
	Status              string     `json:"status"`
	LeaseOwner          string     `json:"leaseOwner"`
	LeaseExpiresAt      *time.Time `json:"leaseExpiresAt"`
	CompletedAt         *time.Time `json:"completedAt"`
	Version             int64      `json:"version"`
	CreatedAt           time.Time  `json:"createdAt"`
	UpdatedAt           time.Time  `json:"updatedAt"`
}

// TaskClaimInput claims a task. ClientClaimID makes a retried claim by the
// same worker idempotent.
type TaskClaimInput struct {
	ClientClaimID string `json:"clientClaimId"`
}

// PutawayCompleteInput confirms the put-away scan.
type PutawayCompleteInput struct {
	ToLocationID        string  `json:"toLocationId"`
	ScannedLocationCode string  `json:"scannedLocationCode"`
	ScannedSKU          string  `json:"scannedSku"`
	Quantity            float64 `json:"quantity"`
}

// PickWave groups released fulfilment orders for picking.
type PickWave struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organizationId"`
	WarehouseID    string     `json:"warehouseId"`
	WaveNumber     string     `json:"waveNumber"`
	Status         string     `json:"status"`
	CompletedAt    *time.Time `json:"completedAt"`
	Version        int64      `json:"version"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	Tasks          []PickTask `json:"tasks"`
}

// PickWaveInput creates a wave from released fulfilment orders.
type PickWaveInput struct {
	WarehouseID        string   `json:"warehouseId"`
	WaveNumber         string   `json:"waveNumber"`
	FulfilmentOrderIDs []string `json:"fulfilmentOrderIds"`
}

// PickTask picks one reserved fulfilment line.
type PickTask struct {
	ID                string     `json:"id"`
	OrganizationID    string     `json:"organizationId"`
	PickWaveID        string     `json:"pickWaveId"`
	FulfilmentOrderID string     `json:"fulfilmentOrderId"`
	FulfilmentLineID  string     `json:"fulfilmentLineId"`
	InventoryItemID   string     `json:"inventoryItemId"`
	FromLocationID    string     `json:"fromLocationId"`
	Quantity          float64    `json:"quantity"`
	PickedQuantity    float64    `json:"pickedQuantity"`
	Status            string     `json:"status"`
	LeaseOwner        string     `json:"leaseOwner"`
	LeaseExpiresAt    *time.Time `json:"leaseExpiresAt"`
	CompletedAt       *time.Time `json:"completedAt"`
	Version           int64      `json:"version"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
}

// PickCompleteInput confirms a pick scan. A picked quantity below the task
// quantity records a short pick and releases the unpicked reservation.
type PickCompleteInput struct {
	ScannedLocationCode string  `json:"scannedLocationCode"`
	ScannedSKU          string  `json:"scannedSku"`
	PickedQuantity      float64 `json:"pickedQuantity"`
}

// PackingUnit is one sealed or open carton/pallet for a fulfilment order.
type PackingUnit struct {
	ID                string          `json:"id"`
	OrganizationID    string          `json:"organizationId"`
	FulfilmentOrderID string          `json:"fulfilmentOrderId"`
	PackingCode       string          `json:"packingCode"`
	Status            string          `json:"status"`
	WeightGrams       *int64          `json:"weightGrams"`
	DimensionsMm      json.RawMessage `json:"dimensionsMm"`
	SealNumber        string          `json:"sealNumber"`
	Contents          []PackedContent `json:"contents"`
	PackedAt          time.Time       `json:"packedAt"`
	Version           int64           `json:"version"`
	CreatedAt         time.Time       `json:"createdAt"`
}

// PackedContent is one picked item quantity inside a packing unit.
type PackedContent struct {
	InventoryItemID string  `json:"inventoryItemId"`
	Quantity        float64 `json:"quantity"`
}

// GateEvent is an append-only yard gate record.
type GateEvent struct {
	ID              string          `json:"id"`
	OrganizationID  string          `json:"organizationId"`
	YardVisitID     string          `json:"yardVisitId"`
	SequenceNo      int             `json:"sequenceNo"`
	EventType       string          `json:"eventType"`
	GateCode        string          `json:"gateCode"`
	ActorReference  string          `json:"actorReference"`
	EvidenceFileIDs json.RawMessage `json:"evidenceFileIds"`
	Notes           string          `json:"notes"`
	OccurredAt      time.Time       `json:"occurredAt"`
}

// GateEventInput records a gate event.
type GateEventInput struct {
	EventType       string   `json:"eventType"`
	GateCode        string   `json:"gateCode"`
	EvidenceFileIDs []string `json:"evidenceFileIds"`
	Notes           string   `json:"notes"`
}

// LocationRef is the identity of a storage location used in scan checks.
type LocationRef struct {
	ID, WarehouseID, Code, Type, Status string
}

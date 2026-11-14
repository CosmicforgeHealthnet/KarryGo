// Package specialized owns organization-scoped extension data for Section 20.
// It deliberately references shared orders, shipments, assets, trips and
// financial authorities instead of duplicating them.
package specialized

import (
	"encoding/json"
	"time"
)

const (
	Courier      = "COURIER_LAST_MILE"
	Taxi         = "TAXI_PASSENGER"
	Hauling      = "TRUCKING_HAULAGE"
	Heavy        = "HEAVY_SPECIALIZED"
	Freight      = "FREIGHT_FORWARDING"
	ColdChain    = "COLD_CHAIN"
	Medical      = "MEDICAL_SENSITIVE"
	Agricultural = "AGRICULTURAL_FOOD"
	Moving       = "MOVING_RELOCATION"
	Towing       = "TOWING_RECOVERY"
)

// SubjectOrder is the subject type of order-level specialized data
// (spec 50.3 /orders/{id}/specialized-data).
const SubjectOrder = "ORDER"

type Flags struct {
	Enabled, Workers, KillSwitch bool
	Modules                      map[string]bool
}
type Capability struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organizationId"`
	CapabilityCode string          `json:"capabilityCode"`
	Status         string          `json:"status"`
	Configuration  json.RawMessage `json:"configuration"`
	EnabledAt      *time.Time      `json:"enabledAt"`
	DisabledAt     *time.Time      `json:"disabledAt"`
	Version        int64           `json:"version"`
	CreatedAt      time.Time       `json:"createdAt"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}
type Record struct {
	ID                         string          `json:"id"`
	OrganizationID             string          `json:"organizationId"`
	BranchID                   string          `json:"branchId"`
	CapabilityCode             string          `json:"capabilityCode"`
	RecordType                 string          `json:"recordType"`
	SubjectType                string          `json:"subjectType"`
	SubjectReference           string          `json:"subjectReference"`
	Status                     string          `json:"status"`
	RestrictedVisibility       bool            `json:"restrictedVisibility"`
	Payload                    json.RawMessage `json:"payload"`
	ExternalFinancialReference string          `json:"externalFinancialReference"`
	SourceAuthority            string          `json:"sourceAuthority"`
	SourceReference            string          `json:"sourceReference"`
	IdempotencyKey             string          `json:"idempotencyKey"`
	Version                    int64           `json:"version"`
	CreatedBy                  string          `json:"createdBy"`
	CreatedAt                  time.Time       `json:"createdAt"`
	UpdatedAt                  time.Time       `json:"updatedAt"`
	CompletedAt                *time.Time      `json:"completedAt"`
	// SchemaVersion is the extension schema version the payload was
	// validated against; SchemaID names it (e.g. COLD_CHAIN.v2).
	SchemaVersion int    `json:"schemaVersion"`
	SchemaID      string `json:"schemaId"`
}
type Milestone struct {
	ID             string          `json:"id"`
	OrganizationID string          `json:"organizationId"`
	RecordID       string          `json:"recordId"`
	MilestoneType  string          `json:"milestoneType"`
	Status         string          `json:"status"`
	SafeMetadata   json.RawMessage `json:"safeMetadata"`
	SourceTime     time.Time       `json:"sourceTime"`
	ReceivedAt     time.Time       `json:"receivedAt"`
	ActorReference string          `json:"actorReference"`
	CorrelationID  string          `json:"correlationId"`
}
type TemperatureEvent struct {
	ID              string          `json:"id"`
	OrganizationID  string          `json:"organizationId"`
	RecordID        string          `json:"recordId"`
	SensorReference string          `json:"sensorReference"`
	Temperature     float64         `json:"temperature"`
	Unit            string          `json:"unit"`
	SourceVersion   int64           `json:"sourceVersion"`
	SourceTime      time.Time       `json:"sourceTime"`
	ReceivedAt      time.Time       `json:"receivedAt"`
	Excursion       bool            `json:"excursion"`
	SafeMetadata    json.RawMessage `json:"safeMetadata"`
}

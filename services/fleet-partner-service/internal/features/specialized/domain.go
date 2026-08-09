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

type Flags struct {
	Enabled, Workers, KillSwitch bool
	Modules                      map[string]bool
}
type Capability struct {
	ID, OrganizationID, CapabilityCode, Status string
	Configuration                              json.RawMessage
	EnabledAt, DisabledAt                      *time.Time
	Version                                    int64
	CreatedAt, UpdatedAt                       time.Time
}
type Record struct {
	ID, OrganizationID, BranchID, CapabilityCode, RecordType, SubjectType, SubjectReference, Status string
	RestrictedVisibility                                                                            bool
	Payload                                                                                         json.RawMessage
	ExternalFinancialReference, SourceAuthority, SourceReference, IdempotencyKey                    string
	Version                                                                                         int64
	CreatedBy                                                                                       string
	CreatedAt, UpdatedAt                                                                            time.Time
	CompletedAt                                                                                     *time.Time
}
type Milestone struct {
	ID, OrganizationID, RecordID, MilestoneType, Status string
	SafeMetadata                                        json.RawMessage
	SourceTime, ReceivedAt                              time.Time
	ActorReference, CorrelationID                       string
}
type TemperatureEvent struct {
	ID, OrganizationID, RecordID, SensorReference string
	Temperature                                   float64
	Unit                                          string
	SourceVersion                                 int64
	SourceTime, ReceivedAt                        time.Time
	Excursion                                     bool
	SafeMetadata                                  json.RawMessage
}

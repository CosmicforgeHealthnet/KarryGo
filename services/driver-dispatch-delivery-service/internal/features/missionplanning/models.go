package missionplanning

import (
	"time"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/rollout"
)

const PolicyVersion = "dispatch_multi_vendor_planning_v2"

type Outcome string

const (
	Consolidate        Outcome = "CONSOLIDATE_SINGLE_MISSION"
	Split              Outcome = "SPLIT_MULTIPLE_MISSIONS"
	WaitReadiness      Outcome = "WAIT_FOR_READINESS"
	OperationalReview  Outcome = "OPERATIONAL_REVIEW"
	RejectIncompatible Outcome = "REJECT_INCOMPATIBLE"
)

type Point struct {
	Address   string  `json:"address"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}
type ReadyWindow struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}
type Unit struct {
	UnitRef    string      `json:"unit_ref"`
	VendorRef  string      `json:"vendor_ref"`
	BranchRef  string      `json:"branch_ref"`
	OrderRef   string      `json:"order_ref"`
	Pickup     Point       `json:"pickup"`
	Ready      ReadyWindow `json:"ready"`
	ParcelRefs []string    `json:"parcel_refs"`
}
type Parcel struct {
	ParcelRef            string   `json:"parcel_ref"`
	KarryGoParcelRef     string   `json:"karrygo_parcel_ref"`
	ScannableCode        string   `json:"scannable_code"`
	UnitRef              string   `json:"unit_ref"`
	WeightGrams          int64    `json:"weight_grams"`
	VolumeML             int64    `json:"volume_ml"`
	Temperature          string   `json:"temperature,omitempty"`
	Handling             []string `json:"handling,omitempty"`
	PackagePolicyVersion string   `json:"package_policy_version"`
}
type PlanRequest struct {
	SourceRef           string    `json:"source_ref"`
	SourceVersion       int64     `json:"source_version"`
	CustomerID          string    `json:"customer_id"`
	Receiver            Point     `json:"receiver"`
	ServiceClass        string    `json:"service_class"`
	Units               []Unit    `json:"units"`
	Parcels             []Parcel  `json:"parcels"`
	VehicleClass        string    `json:"vehicle_class"`
	MaxWeightGrams      int64     `json:"max_weight_grams"`
	MaxVolumeML         int64     `json:"max_volume_ml"`
	PromisedBy          time.Time `json:"promised_by"`
	OperationRef        string    `json:"operation_ref"`
	CorrelationID       string    `json:"correlation_id,omitempty"`
	ExpectedPlanVersion int64     `json:"expected_plan_version,omitempty"`
	ReceiverName        string    `json:"receiver_name,omitempty"`
	ReceiverPhone       string    `json:"receiver_phone,omitempty"`
	FareAmount          int64     `json:"fare_amount_kobo,omitempty"`
	Currency            string    `json:"currency,omitempty"`
	PaymentRef          string    `json:"payment_ref,omitempty"`
	ServiceabilityRef   string    `json:"serviceability_ref,omitempty"`
	// RouteEvidence is populated only by the server-owned Section 6 adapter.
	RouteEvidence *RouteEvidence     `json:"-"`
	Protection    rollout.Protection `json:"-"`
}
type RouteEvidence struct {
	Reference       string  `json:"reference"`
	Serviceable     bool    `json:"serviceable"`
	Degraded        bool    `json:"degraded"`
	DistanceKM      float64 `json:"distance_km"`
	DurationSeconds int64   `json:"duration_seconds"`
	ReasonCode      string  `json:"reason_code,omitempty"`
}
type Stop struct {
	StopRef    string      `json:"stop_ref"`
	Sequence   int         `json:"sequence"`
	Type       string      `json:"type"`
	Location   Point       `json:"location"`
	Ready      ReadyWindow `json:"ready,omitempty"`
	ParcelRefs []string    `json:"parcel_refs"`
}
type Mission struct {
	MissionRef        string   `json:"mission_ref"`
	LogicalMissionRef string   `json:"logical_mission_ref"`
	Index             int      `json:"index"`
	Stops             []Stop   `json:"stops"`
	ParcelRefs        []string `json:"parcel_refs"`
	ManifestRef       string   `json:"manifest_ref"`
	ManifestVersion   int64    `json:"manifest_version"`
}
type Plan struct {
	GroupRef                string         `json:"group_ref"`
	PlanRef                 string         `json:"plan_ref"`
	Version                 int64          `json:"version"`
	Outcome                 Outcome        `json:"outcome"`
	PolicyVersion           string         `json:"policy_version"`
	Reasons                 []string       `json:"reason_codes"`
	Missions                []Mission      `json:"missions"`
	Parcels                 []Parcel       `json:"parcels"`
	Replayed                bool           `json:"replayed"`
	CapacityPolicyVersion   string         `json:"capacity_policy_version"`
	EffectiveMaxWeightGrams int64          `json:"effective_max_weight_grams"`
	EffectiveMaxVolumeML    int64          `json:"effective_max_volume_ml"`
	RouteEvidence           *RouteEvidence `json:"route_evidence,omitempty"`
}
type CustodyCommand struct {
	MissionRef      string `json:"mission_ref"`
	ManifestVersion int64  `json:"manifest_version"`
	SourceRef       string `json:"source_ref"`
	SourceVersion   int64  `json:"source_version"`
	OperationRef    string `json:"-"`
}

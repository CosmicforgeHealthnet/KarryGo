// Package dispatchcommerce defines the versioned internal contract between
// Vendor Service and Dispatch for route-aware, multi-pickup commerce delivery.
// It contains wire types and validation only; pricing, routing, assignment and
// custody remain owned by Dispatch.
package dispatchcommerce

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/custodychain"
)

const (
	SignatureCaller = "vendor-service"
	APIBase         = "/api/v1"
	BasePath        = "/internal/dispatch/v1/commerce"
	QuotePath       = BasePath + "/quotes"
	MissionPath     = BasePath + "/missions"
	CapabilityPath  = BasePath + "/capabilities/motorcycle"

	MaxPickupStops = 12
	MaxParcels     = 100
)

type Mode string

const (
	ModeFastest  Mode = "FASTEST"
	ModeBalanced Mode = "BALANCED"
	ModeSaver    Mode = "SAVER"
)

func (m Mode) Valid() bool {
	return m == ModeFastest || m == ModeBalanced || m == ModeSaver
}

type Point struct {
	Ref       string  `json:"ref,omitempty"`
	Address   string  `json:"address"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Contact   string  `json:"contact,omitempty"`
	Phone     string  `json:"phone,omitempty"`
	Notes     string  `json:"notes,omitempty"`
}

func (p Point) valid() bool {
	if math.IsNaN(p.Latitude) || math.IsNaN(p.Longitude) ||
		math.IsInf(p.Latitude, 0) || math.IsInf(p.Longitude, 0) {
		return false
	}
	if p.Latitude < -90 || p.Latitude > 90 || p.Longitude < -180 || p.Longitude > 180 {
		return false
	}
	return (p.Latitude != 0 || p.Longitude != 0) && strings.TrimSpace(p.Address) != ""
}

type ReadyWindow struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

func (w ReadyWindow) valid() bool {
	return !w.From.IsZero() && !w.To.IsZero() && w.To.After(w.From)
}

type Handling struct {
	Fragile         bool     `json:"fragile,omitempty"`
	Upright         bool     `json:"upright,omitempty"`
	SpillLiquid     bool     `json:"spill_liquid,omitempty"`
	Hot             bool     `json:"hot,omitempty"`
	Chilled         bool     `json:"chilled,omitempty"`
	Frozen          bool     `json:"frozen,omitempty"`
	HighValue       bool     `json:"high_value,omitempty"`
	SpecialHandling []string `json:"special_handling,omitempty"`
}

type Parcel struct {
	ParcelRef         string    `json:"parcel_ref"`
	VendorRef         string    `json:"vendor_ref"`
	BranchRef         string    `json:"branch_ref"`
	FulfilmentUnit    string    `json:"fulfilment_unit_ref"`
	VendorOrderRef    string    `json:"vendor_order_ref"`
	PickupStopRef     string    `json:"pickup_stop_ref"`
	ManifestRef       string    `json:"manifest_ref"`
	ManifestVersion   int64     `json:"manifest_version"`
	ManifestCreatedAt time.Time `json:"manifest_created_at,omitempty"`
	Manifest          any       `json:"manifest,omitempty"`
	WeightGrams       int64     `json:"weight_grams"`
	VolumeMl          int64     `json:"volume_ml,omitempty"`
	LengthMm          int64     `json:"length_mm,omitempty"`
	WidthMm           int64     `json:"width_mm,omitempty"`
	HeightMm          int64     `json:"height_mm,omitempty"`
	PackageCount      int64     `json:"package_count"`
	Handling          Handling  `json:"handling"`
}

type PickupStop struct {
	StopRef string `json:"stop_ref"`

	// PhysicalStopRef groups co-located logical vendor obligations into one
	// route visit while StopRef keeps each manifest and handover independent.
	PhysicalStopRef string `json:"physical_stop_ref,omitempty"`
	VendorRef       string `json:"vendor_ref"`
	VendorDisplay   string `json:"vendor_display,omitempty"`
	BranchRef       string `json:"branch_ref"`
	ManifestRef     string `json:"manifest_ref"`
	ManifestVersion int64  `json:"manifest_version"`

	Location    Point       `json:"location"`
	ReadyWindow ReadyWindow `json:"ready_window"`
	ParcelRefs  []string    `json:"parcel_refs"`
	Handling    Handling    `json:"handling"`
}

type VehicleRequirement struct {
	Class              string   `json:"class"`
	CapabilityVersion  int64    `json:"capability_version"`
	MaxWeightGrams     int64    `json:"max_weight_grams,omitempty"`
	MaxVolumeMl        int64    `json:"max_volume_ml,omitempty"`
	LongestDimensionMm int64    `json:"longest_dimension_mm,omitempty"`
	RequiredHandling   []string `json:"required_handling,omitempty"`
}

type QuoteRequest struct {
	QuoteReference string       `json:"quote_reference"`
	Mode           Mode         `json:"mode"`
	ServiceClass   string       `json:"service_class"`
	Plan           *PlanBinding `json:"plan,omitempty"`

	PickupStops []PickupStop       `json:"pickup_stops"`
	Receiver    Point              `json:"receiver"`
	Parcels     []Parcel           `json:"parcels"`
	Vehicle     VehicleRequirement `json:"vehicle_requirement"`

	PromisedBy    time.Time `json:"promised_by"`
	Currency      string    `json:"currency"`
	CorrelationID string    `json:"correlation_id,omitempty"`
}

type RouteStop struct {
	StopRef         string `json:"stop_ref"`
	PhysicalStopRef string `json:"physical_stop_ref"`
	Sequence        int    `json:"sequence"`
	DistanceMeters  int64  `json:"distance_meters"`
	DurationSeconds int64  `json:"duration_seconds"`
}

type Quote struct {
	QuoteID            string `json:"quote_id"`
	QuoteReference     string `json:"quote_reference"`
	Revision           int64  `json:"revision"`
	RequestFingerprint string `json:"request_fingerprint"`
	Serviceable        bool   `json:"serviceable"`
	ReasonCode         string `json:"reason_code"`
	SupplyState        string `json:"supply_state"`

	Mode              Mode   `json:"mode"`
	ServiceClass      string `json:"service_class"`
	VehicleClass      string `json:"vehicle_class"`
	CapabilityVersion int64  `json:"capability_version"`

	RouteProvider        string      `json:"route_provider"`
	RouteEvidenceVersion string      `json:"route_evidence_version"`
	DistanceMeters       int64       `json:"distance_meters"`
	EstimatedSeconds     int64       `json:"estimated_seconds"`
	StopSequence         []RouteStop `json:"stop_sequence"`

	FeeMinor int64  `json:"fee_minor"`
	Currency string `json:"currency,omitempty"`

	AvailableProviders int       `json:"available_providers"`
	ExpiresAt          time.Time `json:"expires_at"`
	QuotedAt           time.Time `json:"quoted_at"`
}

type MotorcycleCapability struct {
	Class           string    `json:"class"`
	Version         int64     `json:"version"`
	MaxWeightGrams  int64     `json:"max_weight_grams"`
	MaxVolumeMl     int64     `json:"max_volume_ml"`
	MaxDimensionMm  int64     `json:"max_dimension_mm"`
	MaxPackageCount int64     `json:"max_package_count"`
	Handling        []string  `json:"handling"`
	MultiPickup     bool      `json:"multi_pickup"`
	MaxPickupStops  int       `json:"max_pickup_stops"`
	ObservedAt      time.Time `json:"observed_at"`
}

type PlanBinding struct {
	PlanRef         string `json:"plan_ref"`
	PlanVersion     int64  `json:"plan_version"`
	PlanFingerprint string `json:"plan_fingerprint"`
	MissionSlotRef  string `json:"mission_slot_ref"`
}

type QuoteBinding struct {
	QuoteID          string `json:"quote_id"`
	QuoteReference   string `json:"quote_reference"`
	QuoteRevision    int64  `json:"quote_revision"`
	QuoteFingerprint string `json:"quote_fingerprint"`
}

type MissionCreateRequest struct {
	OperationRef     string             `json:"operation_ref"`
	ParentOrderRef   string             `json:"parent_order_ref"`
	CustomerRef      string             `json:"customer_ref"`
	Plan             PlanBinding        `json:"plan"`
	Quote            QuoteBinding       `json:"quote"`
	Mode             Mode               `json:"mode"`
	ServiceClass     string             `json:"service_class"`
	PickupStops      []PickupStop       `json:"pickup_stops"`
	Receiver         Point              `json:"receiver"`
	Parcels          []Parcel           `json:"parcels"`
	Vehicle          VehicleRequirement `json:"vehicle_requirement"`
	PromisedBy       time.Time          `json:"promised_by"`
	FundingReference string             `json:"funding_reference"`
	CorrelationID    string             `json:"correlation_id,omitempty"`
}

type StopView struct {
	StopRef         string      `json:"stop_ref"`
	PhysicalStopRef string      `json:"physical_stop_ref"`
	Sequence        int         `json:"sequence"`
	VendorRef       string      `json:"vendor_ref"`
	BranchRef       string      `json:"branch_ref"`
	ManifestRef     string      `json:"manifest_ref"`
	ManifestVersion int64       `json:"manifest_version"`
	ParcelRefs      []string    `json:"parcel_refs"`
	ReadyWindow     ReadyWindow `json:"ready_window"`
	Status          string      `json:"status"`
	HandoverStatus  string      `json:"handover_status"`
	Location        Point       `json:"location"`
	Handling        Handling    `json:"handling"`
}

type ParcelView struct {
	ParcelRef       string `json:"parcel_ref"`
	VendorRef       string `json:"vendor_ref"`
	BranchRef       string `json:"branch_ref"`
	FulfilmentUnit  string `json:"fulfilment_unit_ref"`
	VendorOrderRef  string `json:"vendor_order_ref"`
	PickupStopRef   string `json:"pickup_stop_ref"`
	ManifestRef     string `json:"manifest_ref"`
	ManifestVersion int64  `json:"manifest_version"`
	PackageCount    int64  `json:"package_count"`
	CustodianType   string `json:"custodian_type"`
	CustodianRef    string `json:"custodian_ref,omitempty"`
	Status          string `json:"status"`
}

type Mission struct {
	MissionRef   string       `json:"mission_ref"`
	OperationRef string       `json:"operation_ref"`
	BookingRef   string       `json:"booking_ref,omitempty"`
	TripRef      string       `json:"trip_ref,omitempty"`
	State        string       `json:"state"`
	QueueState   string       `json:"queue_state"`
	Version      int64        `json:"version"`
	Plan         PlanBinding  `json:"plan"`
	Quote        QuoteBinding `json:"quote"`
	Stops        []StopView   `json:"stops"`
	Parcels      []ParcelView `json:"parcels"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
	ReasonCode   string       `json:"reason_code,omitempty"`
	Replayed     bool         `json:"replayed,omitempty"`
}

type CommandRequest struct {
	OperationRef        string     `json:"operation_ref"`
	ExpectedVersion     int64      `json:"expected_version,omitempty"`
	ReasonCode          string     `json:"reason_code,omitempty"`
	ReadyAt             *time.Time `json:"ready_at,omitempty"`
	TransferEvidenceRef string     `json:"transfer_evidence_ref,omitempty"`
	CorrelationID       string     `json:"correlation_id,omitempty"`
}

type PickupVerificationRequest struct {
	OperationRef    string `json:"operation_ref"`
	Code            string `json:"code"`
	ExpectedVersion int64  `json:"expected_version,omitempty"`
}

type PickupCredential struct {
	MissionRef        string    `json:"mission_ref"`
	StopRef           string    `json:"stop_ref"`
	Code              string    `json:"code"`
	ExpiresAt         time.Time `json:"expires_at"`
	AttemptsRemaining int       `json:"attempts_remaining"`

	// Direction says who holds this credential and who may verify it, so the
	// answer does not depend on which route the caller happened to use.
	// Commerce pickup is provider-held and vendor-verified.
	Direction custodychain.Direction `json:"direction"`
}

// WithDirection stamps the canonical Commerce-pickup direction onto the
// credential. Kept as a method so every site that builds one gets the same
// description rather than filling the struct in by hand.
func (c PickupCredential) WithDirection() PickupCredential {
	direction, ok := custodychain.DirectionFor(
		custodychain.PurposeCommercePickup, "DISPATCH",
		c.MissionRef, &c.ExpiresAt)
	if ok {
		c.Direction = direction
	}
	return c
}

type StopActionRequest struct {
	OperationRef    string         `json:"operation_ref"`
	ExpectedVersion int64          `json:"expected_version,omitempty"`
	ManifestRef     string         `json:"manifest_ref,omitempty"`
	ManifestVersion int64          `json:"manifest_version,omitempty"`
	ParcelRefs      []string       `json:"parcel_refs,omitempty"`
	PackageCount    int64          `json:"package_count,omitempty"`
	Evidence        *EvidenceInput `json:"evidence,omitempty"`
}

// EvidenceInput carries evidence metadata only. Media bytes remain owned by
// media-file-service and are represented here by opaque references.
type EvidenceInput struct {
	CapturedAt  time.Time `json:"captured_at"`
	Latitude    *float64  `json:"latitude,omitempty"`
	Longitude   *float64  `json:"longitude,omitempty"`
	MediaRefs   []string  `json:"media_refs,omitempty"`
	Description string    `json:"description,omitempty"`
}

type JobCard struct {
	Commerce          bool       `json:"commerce"`
	MissionRef        string     `json:"mission_ref"`
	ServiceClass      string     `json:"service_class"`
	Stops             []StopView `json:"pickup_stops"`
	Receiver          Point      `json:"receiver"`
	ParcelCount       int        `json:"parcel_count"`
	ProofRequirements []string   `json:"proof_requirements"`
}

type Envelope[T any] struct {
	Success bool `json:"success"`
	Data    T    `json:"data"`
	Error   struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (r QuoteRequest) Validate() (string, bool) {
	if strings.TrimSpace(r.QuoteReference) == "" || !r.Mode.Valid() {
		return "quote_reference and valid mode are required", false
	}
	if len(r.PickupStops) < 1 || len(r.PickupStops) > MaxPickupStops || !r.Receiver.valid() {
		return "one to twelve pickups and one valid receiver are required", false
	}
	if len(r.Parcels) < 1 || len(r.Parcels) > MaxParcels || len(strings.TrimSpace(r.Currency)) != 3 {
		return "one to one hundred parcels and a currency are required", false
	}
	if !r.PromisedBy.IsZero() && !r.PromisedBy.After(time.Now().Add(-time.Minute)) {
		return "promised_by is stale", false
	}
	return validateStopsAndParcels(r.PickupStops, r.Parcels, r.Receiver, r.Vehicle)
}

func (r MissionCreateRequest) Validate() (string, bool) {
	if strings.TrimSpace(r.OperationRef) == "" || strings.TrimSpace(r.ParentOrderRef) == "" ||
		strings.TrimSpace(r.CustomerRef) == "" || strings.TrimSpace(r.Plan.PlanRef) == "" ||
		r.Plan.PlanVersion < 1 || strings.TrimSpace(r.Plan.PlanFingerprint) == "" ||
		strings.TrimSpace(r.Plan.MissionSlotRef) == "" || strings.TrimSpace(r.Quote.QuoteID) == "" ||
		r.Quote.QuoteRevision < 1 || strings.TrimSpace(r.Quote.QuoteFingerprint) == "" || !r.Mode.Valid() {
		return "operation, immutable plan, quote and mode are required", false
	}
	if len(r.PickupStops) < 1 || len(r.PickupStops) > MaxPickupStops ||
		len(r.Parcels) < 1 || len(r.Parcels) > MaxParcels {
		return "pickup or parcel bounds exceeded", false
	}
	return validateStopsAndParcels(r.PickupStops, r.Parcels, r.Receiver, r.Vehicle)
}

func validateStopsAndParcels(stops []PickupStop, parcels []Parcel, receiver Point, vehicle VehicleRequirement) (string, bool) {
	if !receiver.valid() || !strings.EqualFold(strings.TrimSpace(vehicle.Class), "MOTORCYCLE") || vehicle.CapabilityVersion < 1 {
		return "receiver and motorcycle capability are required", false
	}
	stopRefs := make(map[string]bool, len(stops))
	for _, stop := range stops {
		ref := strings.TrimSpace(stop.StopRef)
		if ref == "" || stopRefs[ref] || strings.TrimSpace(stop.VendorRef) == "" ||
			strings.TrimSpace(stop.BranchRef) == "" || strings.TrimSpace(stop.ManifestRef) == "" ||
			stop.ManifestVersion < 1 ||
			!stop.Location.valid() || !stop.ReadyWindow.valid() || len(stop.ParcelRefs) == 0 {
			return "pickup stop is incomplete or duplicated", false
		}
		stopRefs[ref] = true
	}
	parcelRefs := make(map[string]bool, len(parcels))
	for _, parcel := range parcels {
		ref := strings.TrimSpace(parcel.ParcelRef)
		if ref == "" || parcelRefs[ref] || !stopRefs[strings.TrimSpace(parcel.PickupStopRef)] ||
			strings.TrimSpace(parcel.VendorRef) == "" || strings.TrimSpace(parcel.BranchRef) == "" ||
			strings.TrimSpace(parcel.FulfilmentUnit) == "" || strings.TrimSpace(parcel.VendorOrderRef) == "" ||
			strings.TrimSpace(parcel.ManifestRef) == "" || parcel.ManifestVersion < 1 || parcel.WeightGrams < 0 || parcel.VolumeMl < 0 ||
			parcel.LengthMm < 0 || parcel.WidthMm < 0 || parcel.HeightMm < 0 || parcel.PackageCount < 1 {
			return "parcel is incomplete, duplicated or references an unknown stop", false
		}
		parcelRefs[ref] = true
	}
	for _, stop := range stops {
		seen := map[string]bool{}
		for _, parcelRef := range stop.ParcelRefs {
			parcelRef = strings.TrimSpace(parcelRef)
			if seen[parcelRef] || !parcelRefs[parcelRef] {
				return "stop parcel references are duplicated or unknown", false
			}
			seen[parcelRef] = true
		}
	}
	return "", true
}

// Fingerprint hashes semantic request content after deterministic ordering.
// Correlation IDs and transport timestamps are excluded by callers before use.
func Fingerprint(value any) (string, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func SortedParcelRefs(parcels []ParcelView) []string {
	refs := make([]string, 0, len(parcels))
	for _, parcel := range parcels {
		refs = append(refs, parcel.ParcelRef)
	}
	sort.Strings(refs)
	return refs
}

// Package commercemission is the internal contract for creating a logistics
// mission from an already-paid commerce order.
//
// It is the execution counterpart to shared/go/commercequote. That package
// asks a logistics authority what a delivery would cost; this one tells the
// authority to actually run it, for a fee the customer has ALREADY been
// charged under an approved quote.
//
// The single most important rule here: the logistics authority does not
// re-price. AgreedFeeMinor is what the customer already paid, and the mission
// is created for that figure. If the authority's own current price differs,
// that difference is an internal platform cost and never reaches the customer.
// A contract that let the executor quote again would silently break the total
// price guarantee.
//
// Schema and client only. Neither this package nor its client ever derives a
// fare, and no logistics service is required to accept a mission it cannot
// serve.
package commercemission

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// SignatureCaller is the single service identity allowed to create commerce
// missions. Each logistics service allowlists exactly this caller.
const SignatureCaller = "vendor-service"

// APIBase paths, mirroring the commerce quote contract's shape.
const (
	DispatchAPIBase = "/api/v1"
	TaxiAPIBase     = "/api/v1/taxi"
	HaulingAPIBase  = "/api/v1/hauling"

	// Path is appended to the authority's own internal commerce base.
	Path = "/internal/commerce/missions"

	OperationPath = Path + "/operations"
)

// Service names the logistics authority that will execute the mission.
//
// Taxi names the distinct Taxi Goods authority. Feature flags and readiness
// still decide whether it can be called; this value never denotes a passenger
// booking.
type Service string

const (
	ServiceDispatch Service = "DISPATCH"
	ServiceTaxi     Service = "TAXI"
	ServiceHauling  Service = "HAULING"
)

// Valid reports whether the service is one this contract can execute.
func (s Service) Valid() bool {
	return s == ServiceDispatch || s == ServiceTaxi || s == ServiceHauling
}

// VehicleClass mirrors the quote contract's vocabulary so a mission is created
// for the class that was quoted.
type VehicleClass string

const (
	VehicleMotorcycle VehicleClass = "MOTORCYCLE"
	VehicleCar        VehicleClass = "CAR"
	VehicleTricycle   VehicleClass = "TRICYCLE"
	VehicleVan        VehicleClass = "VAN"
	VehicleTruck      VehicleClass = "TRUCK"
)

// Valid reports whether the vehicle class is known.
func (v VehicleClass) Valid() bool {
	switch v {
	case VehicleMotorcycle, VehicleTricycle, VehicleCar, VehicleVan, VehicleTruck:
		return true
	default:
		return false
	}
}

// Status is the authority's own answer about a mission it now owns.
type Status string

const (
	// StatusAccepted means the authority has taken ownership and will execute.
	StatusAccepted Status = "ACCEPTED"

	// StatusRefused means the authority will not execute this mission. It is a
	// real answer, not a failure: commerce must handle it rather than retry
	// forever.
	StatusRefused Status = "REFUSED"

	StatusCancelled Status = "CANCELLED"
)

// Point is a pickup or dropoff location.
type Point struct {
	Ref       string  `json:"ref,omitempty"`
	Address   string  `json:"address"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Contact   string  `json:"contact,omitempty"`
	Phone     string  `json:"phone,omitempty"`
	Notes     string  `json:"notes,omitempty"`
}

// Load describes what is being moved.
type Load struct {
	WeightGrams            int64  `json:"weight_grams"`
	PackageCount           int64  `json:"package_count"`
	LoadingDurationSeconds int64  `json:"loading_duration_seconds,omitempty"`
	Description            string `json:"description,omitempty"`
	Fragile                bool   `json:"fragile,omitempty"`
}

const MaxLoadingDurationSeconds int64 = 8 * 60 * 60

type ReadyWindow struct {
	From     time.Time `json:"from,omitempty"`
	To       time.Time `json:"to,omitempty"`
	Timezone string    `json:"timezone,omitempty"`
}

type PlanBinding struct {
	PlanRef         string `json:"plan_ref,omitempty"`
	PlanVersion     int64  `json:"plan_version,omitempty"`
	PlanFingerprint string `json:"plan_fingerprint,omitempty"`
	MissionSlotRef  string `json:"mission_slot_ref,omitempty"`
}

type Handling struct {
	Fragile         bool     `json:"fragile,omitempty"`
	Upright         bool     `json:"upright,omitempty"`
	StackRestricted bool     `json:"stack_restricted,omitempty"`
	SpillLiquid     bool     `json:"spill_liquid,omitempty"`
	Hot             bool     `json:"hot,omitempty"`
	Chilled         bool     `json:"chilled,omitempty"`
	Frozen          bool     `json:"frozen,omitempty"`
	HighValue       bool     `json:"high_value,omitempty"`
	SpecialHandling []string `json:"special_handling,omitempty"`
}

type PickupStop struct {
	StopRef         string      `json:"stop_ref"`
	PhysicalStopRef string      `json:"physical_stop_ref,omitempty"`
	VendorRef       string      `json:"vendor_ref,omitempty"`
	BranchRef       string      `json:"branch_ref,omitempty"`
	ManifestRef     string      `json:"manifest_ref,omitempty"`
	ManifestVersion int64       `json:"manifest_version,omitempty"`
	Location        Point       `json:"location"`
	ReadyWindow     ReadyWindow `json:"ready_window,omitempty"`
	ParcelRefs      []string    `json:"parcel_refs,omitempty"`
	Handling        Handling    `json:"handling,omitempty"`
}

type Parcel struct {
	ParcelRef       string          `json:"parcel_ref"`
	VendorRef       string          `json:"vendor_ref,omitempty"`
	BranchRef       string          `json:"branch_ref,omitempty"`
	FulfilmentUnit  string          `json:"fulfilment_unit,omitempty"`
	VendorOrderRef  string          `json:"vendor_order_ref,omitempty"`
	PickupStopRef   string          `json:"pickup_stop_ref,omitempty"`
	ManifestRef     string          `json:"manifest_ref,omitempty"`
	ManifestVersion int64           `json:"manifest_version,omitempty"`
	Manifest        json.RawMessage `json:"manifest,omitempty"`
	WeightGrams     int64           `json:"weight_grams,omitempty"`
	LengthMm        int64           `json:"length_mm,omitempty"`
	WidthMm         int64           `json:"width_mm,omitempty"`
	HeightMm        int64           `json:"height_mm,omitempty"`
	PackageCount    int64           `json:"package_count,omitempty"`
	Handling        Handling        `json:"handling,omitempty"`
}

type VehicleRequirement struct {
	Class              VehicleClass `json:"class,omitempty"`
	MaxWeightGrams     int64        `json:"max_weight_grams,omitempty"`
	MaxVolumeMl        int64        `json:"max_volume_ml,omitempty"`
	LongestDimensionMm int64        `json:"longest_dimension_mm,omitempty"`
}

// Request asks a logistics authority to execute one commerce mission.
type Request struct {
	// MissionRef is Vendor Service's own mission identity and the idempotency
	// key. Re-sending the same MissionRef must return the same mission rather
	// than creating a second one.
	MissionRef string `json:"mission_ref"`

	OperationRef string `json:"operation_ref,omitempty"`

	ParentOrderRef string `json:"parent_order_ref"`

	// VendorOrderRefs are every vendor allocation this mission covers. A
	// consolidated mission names more than one.
	VendorOrderRefs []string `json:"vendor_order_refs"`

	CustomerRef string `json:"customer_ref"`

	Service      Service      `json:"service"`
	VehicleClass VehicleClass `json:"vehicle_class,omitempty"`

	// QuoteID and QuoteReference are the authority's OWN earlier quote, echoed
	// back so it can tie execution to what it priced.
	QuoteID        string `json:"quote_id,omitempty"`
	QuoteReference string `json:"quote_reference,omitempty"`

	Plan        *PlanBinding       `json:"plan,omitempty"`
	Pickup      Point              `json:"pickup"`
	PickupStops []PickupStop       `json:"pickup_stops,omitempty"`
	Dropoff     Point              `json:"dropoff"`
	Parcels     []Parcel           `json:"parcels,omitempty"`
	Vehicle     VehicleRequirement `json:"vehicle,omitempty"`
	Load        Load               `json:"load"`
	Handling    Handling           `json:"handling,omitempty"`

	Currency string `json:"currency"`

	// AgreedFeeMinor is what the customer has ALREADY paid for this delivery
	// under an approved quote. The authority executes for this figure and does
	// not re-price. Any difference from its own current price is an internal
	// platform cost.
	AgreedFeeMinor int64 `json:"agreed_fee_minor"`

	RequestedAt time.Time `json:"requested_at"`
}

func (r Request) OperationReference() string {
	if value := strings.TrimSpace(r.OperationRef); value != "" {
		return value
	}
	return strings.TrimSpace(r.MissionRef)
}

func (r Request) Fingerprint() string {
	payload := struct {
		OperationRef    string             `json:"operation_ref"`
		MissionRef      string             `json:"mission_ref"`
		ParentOrderRef  string             `json:"parent_order_ref"`
		VendorOrderRefs []string           `json:"vendor_order_refs"`
		CustomerRef     string             `json:"customer_ref"`
		Service         Service            `json:"service"`
		VehicleClass    VehicleClass       `json:"vehicle_class"`
		QuoteID         string             `json:"quote_id"`
		QuoteReference  string             `json:"quote_reference"`
		Plan            *PlanBinding       `json:"plan,omitempty"`
		Pickup          Point              `json:"pickup"`
		PickupStops     []PickupStop       `json:"pickup_stops,omitempty"`
		Dropoff         Point              `json:"dropoff"`
		Parcels         []Parcel           `json:"parcels,omitempty"`
		Vehicle         VehicleRequirement `json:"vehicle,omitempty"`
		Load            Load               `json:"load"`
		Handling        Handling           `json:"handling,omitempty"`
		Currency        string             `json:"currency"`
		AgreedFeeMinor  int64              `json:"agreed_fee_minor"`
		RequestedAt     time.Time          `json:"requested_at"`
	}{
		OperationRef: r.OperationReference(), MissionRef: strings.TrimSpace(r.MissionRef),
		ParentOrderRef: strings.TrimSpace(r.ParentOrderRef), VendorOrderRefs: r.VendorOrderRefs,
		CustomerRef: strings.TrimSpace(r.CustomerRef), Service: r.Service, VehicleClass: r.VehicleClass,
		QuoteID: strings.TrimSpace(r.QuoteID), QuoteReference: strings.TrimSpace(r.QuoteReference),
		Plan: r.Plan, Pickup: r.Pickup, PickupStops: r.PickupStops, Dropoff: r.Dropoff,
		Parcels: r.Parcels, Vehicle: r.Vehicle, Load: r.Load, Handling: r.Handling,
		Currency: strings.ToUpper(strings.TrimSpace(r.Currency)), AgreedFeeMinor: r.AgreedFeeMinor,
		RequestedAt: r.RequestedAt.UTC(),
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "unfingerprintable"
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])
}

// Mission is the authority's answer.
type Mission struct {
	MissionRef string `json:"mission_ref"`

	OperationRef string `json:"operation_ref,omitempty"`

	// ExternalRef is the authority's own booking or trip identity, so commerce
	// can follow execution without knowing that service's internals.
	ExternalRef string `json:"external_ref"`

	Status Status `json:"status"`

	Version int64 `json:"version,omitempty"`

	// RefusalCode is required when Status is REFUSED, so commerce can act on
	// the reason rather than retrying blindly.
	RefusalCode string `json:"refusal_code,omitempty"`

	// AcceptedFeeMinor must equal the request's AgreedFeeMinor. It is echoed
	// so the caller can assert the authority did not re-price.
	AcceptedFeeMinor int64 `json:"accepted_fee_minor"`

	AcceptedAt time.Time `json:"accepted_at"`
}

type CancelRequest struct {
	OperationRef    string `json:"operation_ref,omitempty"`
	MissionRef      string `json:"mission_ref,omitempty"`
	ReasonCode      string `json:"reason_code,omitempty"`
	ExpectedVersion int64  `json:"expected_version,omitempty"`
}

// Envelope is the standard platform response shape.
type Envelope struct {
	Success bool    `json:"success"`
	Data    Mission `json:"data"`
	Error   struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

var (
	// ErrInvalidRequest means the request could never be executed as stated.
	ErrInvalidRequest = errors.New("commerce mission request is invalid")

	// ErrRepriced means the authority answered with a fee other than the one
	// the customer already paid. It is refused rather than accepted, because
	// accepting it would silently break the total price guarantee.
	ErrRepriced = errors.New("logistics authority returned a different fee than the agreed one")

	// ErrMissionMismatch means the answer does not belong to the request.
	ErrMissionMismatch = errors.New("logistics authority answered about a different mission")

	ErrAuthorityUnavailable = errors.New("logistics authority is unavailable")

	ErrNotConfigured = errors.New("logistics authority is not configured")
)

// Validate reports the first reason a request could not be executed.
//
// It is exported so the receiving service validates with exactly the same
// rules the sender does, rather than each end inventing its own.
func (r Request) Validate() (string, bool) {
	switch {
	case strings.TrimSpace(r.MissionRef) == "":
		return "mission_ref is required", false
	case strings.TrimSpace(r.ParentOrderRef) == "":
		return "parent_order_ref is required", false
	case len(r.VendorOrderRefs) == 0:
		return "vendor_order_refs must name at least one allocation", false
	case strings.TrimSpace(r.CustomerRef) == "":
		return "customer_ref is required", false
	case !r.Service.Valid():
		return "service must be DISPATCH, TAXI, or HAULING", false
	case r.VehicleClass != "" && !r.VehicleClass.Valid():
		return "vehicle_class is not a known class", false
	case len(r.PickupStops) == 0 && !validCoordinate(r.Pickup.Latitude, r.Pickup.Longitude):
		return "pickup coordinates are out of range", false
	case !validCoordinate(r.Dropoff.Latitude, r.Dropoff.Longitude):
		return "dropoff coordinates are out of range", false
	case strings.TrimSpace(r.Pickup.Address) == "":
		return "pickup.address is required", false
	case strings.TrimSpace(r.Dropoff.Address) == "":
		return "dropoff.address is required", false
	case len(r.Currency) != 3:
		return "currency must be a three letter code", false
	case r.AgreedFeeMinor < 0:
		return "agreed_fee_minor cannot be negative", false
	case r.Load.PackageCount < 0 || r.Load.WeightGrams < 0:
		return "load values cannot be negative", false
	case r.Load.LoadingDurationSeconds < 0 ||
		r.Load.LoadingDurationSeconds > MaxLoadingDurationSeconds:
		return "load.loading_duration_seconds must be between 0 and 28800", false
	}
	if len(r.PickupStops) > 0 {
		seenStops := map[string]bool{}
		seenParcels := map[string]bool{}
		for _, parcel := range r.Parcels {
			ref := strings.TrimSpace(parcel.ParcelRef)
			if ref == "" {
				return "parcel_ref is required", false
			}
			if seenParcels[ref] {
				return "parcel_refs must be unique", false
			}
			seenParcels[ref] = true
			if parcel.WeightGrams < 0 || parcel.PackageCount < 0 {
				return "parcel measurements cannot be negative", false
			}
		}
		for _, stop := range r.PickupStops {
			ref := strings.TrimSpace(stop.StopRef)
			if ref == "" {
				return "pickup stop ref is required", false
			}
			if seenStops[ref] {
				return "pickup stop refs must be unique", false
			}
			seenStops[ref] = true
			if !validCoordinate(stop.Location.Latitude, stop.Location.Longitude) {
				return "pickup stop coordinates are out of range", false
			}
			for _, parcelRef := range stop.ParcelRefs {
				if !seenParcels[strings.TrimSpace(parcelRef)] {
					return "pickup stop references an unknown parcel", false
				}
			}
			if !stop.ReadyWindow.From.IsZero() || !stop.ReadyWindow.To.IsZero() {
				if stop.ReadyWindow.From.IsZero() || stop.ReadyWindow.To.IsZero() ||
					stop.ReadyWindow.To.Before(stop.ReadyWindow.From) {
					return "pickup ready window is invalid", false
				}
				if strings.TrimSpace(stop.ReadyWindow.Timezone) == "" {
					return "pickup ready window timezone is required", false
				}
			}
		}
	}
	return "", true
}

// VerifyMission checks an authority's answer against the request.
//
// The re-pricing check is the point of this function: an authority that
// answers with a different fee is refused, because the customer's charge is
// already fixed by an approved quote.
func VerifyMission(request Request, mission Mission) error {
	if mission.MissionRef != request.MissionRef {
		return ErrMissionMismatch
	}
	switch mission.Status {
	case StatusRefused:
		if strings.TrimSpace(mission.RefusalCode) == "" {
			return ErrInvalidRequest
		}
		return nil
	case StatusAccepted, StatusCancelled:
	default:
		return ErrInvalidRequest
	}
	if strings.TrimSpace(mission.ExternalRef) == "" {
		return ErrInvalidRequest
	}
	if mission.AcceptedFeeMinor != request.AgreedFeeMinor {
		return ErrRepriced
	}
	return nil
}

func validCoordinate(latitude, longitude float64) bool {
	if latitude == 0 && longitude == 0 {
		return false
	}
	return latitude >= -90 && latitude <= 90 && longitude >= -180 && longitude <= 180
}

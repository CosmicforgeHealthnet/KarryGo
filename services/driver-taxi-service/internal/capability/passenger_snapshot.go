package capability

import (
	"errors"
	"strings"
	"time"
)

// Requirement sources recorded per hard requirement so an operator can see
// why a booking carried a gate.
const (
	SourceRequest         = "REQUEST"
	SourceCustomerProfile = "CUSTOMER_PROFILE"
	SourceDefault         = "DEFAULT"

	CustomerContextUsed         = "USED"
	CustomerContextNotRequested = "NOT_REQUESTED"
	CustomerContextUnavailable  = "UNAVAILABLE"

	// GuaranteeProfileNone is recorded when the product declares no guarantee
	// profile. It is an explicit absence, not a placeholder version.
	GuaranteeProfileNone = "NONE"

	PassengerSnapshotSchemaVersion = 1
)

var (
	ErrUnknownProduct         = errors.New("service product is not offered")
	ErrProductPurposeMismatch = errors.New("service product is not a passenger product")
	ErrProductClassMismatch   = errors.New("service product does not match the vehicle class")
	ErrProductPassengerLimit  = errors.New("passenger count exceeds the service product limit")
)

// HardRequirementInput is what the booking request states explicitly. A nil
// field means "not stated", which lets a saved customer need fill it in.
type HardRequirementInput struct {
	WheelchairRequired *bool
	ChildSeatRequired  *bool
	PetTravel          *bool
}

// RidePreferenceInput holds soft preferences. They never gate matching.
type RidePreferenceInput struct {
	QuietRide        *bool
	PreferredChannel string
	Luggage          string
}

// CustomerMobilityHints is the minimized customer context Taxi receives from
// customer-service. Free-text assistance notes are deliberately absent.
type CustomerMobilityHints struct {
	Status              string
	Version             int64
	WheelchairRequired  *bool
	ChildSeatPreference *string
	PetTravelPreference *string
	LuggageDefault      string
	QuietRide           *bool
	PreferredChannel    string
}

// RidePreferences is the frozen soft-preference block of a snapshot.
type RidePreferences struct {
	QuietRide        bool   `json:"quiet_ride"`
	PreferredChannel string `json:"preferred_channel,omitempty"`
	Luggage          string `json:"luggage,omitempty"`
}

type CustomerContextRef struct {
	Status  string `json:"status"`
	Version int64  `json:"version,omitempty"`
}

// PassengerSnapshot is the immutable requirement snapshot persisted on a
// passenger booking. The embedded PassengerRequirement carries the hard
// gates; Preferences are informational only.
type PassengerSnapshot struct {
	SchemaVersion           int    `json:"schema_version"`
	ProductVersion          int64  `json:"product_version"`
	PolicyVersion           string `json:"eligibility_policy_version"`
	GuaranteeProfileVersion string `json:"guarantee_profile_version"`
	PassengerRequirement
	Sources         map[string]string  `json:"requirement_sources"`
	Preferences     RidePreferences    `json:"preferences"`
	CustomerContext CustomerContextRef `json:"customer_context"`
	FrozenAt        time.Time          `json:"frozen_at"`
}

// BuildPassengerSnapshot resolves the product for a passenger booking and
// freezes its hard requirements. An explicit request value always wins over a
// saved customer need; a saved need wins over the default "not required".
func BuildPassengerSnapshot(policy Policy, productCode, vehicleClass string, passengers int, hard HardRequirementInput, prefs RidePreferenceInput, hints CustomerMobilityHints, now time.Time) (PassengerSnapshot, Product, error) {
	code := strings.ToUpper(strings.TrimSpace(productCode))
	if code == "" {
		code = LegacyProductForClass(vehicleClass)
	}
	product, ok := policy.Product(code)
	if !ok || !product.Active {
		return PassengerSnapshot{}, Product{}, ErrUnknownProduct
	}
	if product.Purpose != PurposePassenger {
		return PassengerSnapshot{}, Product{}, ErrProductPurposeMismatch
	}
	class := strings.ToLower(strings.TrimSpace(vehicleClass))
	if legacy := strings.ToLower(strings.TrimSpace(product.LegacyVehicleClass)); legacy != "" && class != "" && legacy != class {
		return PassengerSnapshot{}, Product{}, ErrProductClassMismatch
	}
	if passengers < 1 {
		passengers = 1
	}
	if product.MaxPassengers > 0 && passengers > product.MaxPassengers {
		return PassengerSnapshot{}, Product{}, ErrProductPassengerLimit
	}
	sources := map[string]string{}
	resolve := func(field string, explicit *bool, saved *bool) bool {
		switch {
		case explicit != nil:
			sources[field] = SourceRequest
			return *explicit
		case saved != nil:
			sources[field] = SourceCustomerProfile
			return *saved
		}
		sources[field] = SourceDefault
		return false
	}
	requirement := PassengerRequirement{ProductCode: product.Code, PassengerCount: passengers}
	requirement.WheelchairRequired = resolve("wheelchair_required", hard.WheelchairRequired, hints.WheelchairRequired)
	requirement.ChildSeatRequired = resolve("child_seat_required", hard.ChildSeatRequired, enumNeed(hints.ChildSeatPreference, "NOT_NEEDED"))
	requirement.PetTravel = resolve("pet_travel", hard.PetTravel, enumNeed(hints.PetTravelPreference, "NONE"))

	preferences := RidePreferences{PreferredChannel: strings.ToUpper(strings.TrimSpace(prefs.PreferredChannel)), Luggage: strings.ToUpper(strings.TrimSpace(prefs.Luggage))}
	switch {
	case prefs.QuietRide != nil:
		preferences.QuietRide = *prefs.QuietRide
	case hints.QuietRide != nil:
		preferences.QuietRide = *hints.QuietRide
	}
	if preferences.PreferredChannel == "" {
		preferences.PreferredChannel = strings.ToUpper(strings.TrimSpace(hints.PreferredChannel))
	}
	if preferences.Luggage == "" {
		preferences.Luggage = strings.ToUpper(strings.TrimSpace(hints.LuggageDefault))
	}
	contextStatus := hints.Status
	if contextStatus == "" {
		contextStatus = CustomerContextNotRequested
	}
	guarantee := strings.TrimSpace(product.GuaranteeProfileVersion)
	if guarantee == "" {
		guarantee = GuaranteeProfileNone
	}
	return PassengerSnapshot{
		SchemaVersion: PassengerSnapshotSchemaVersion, ProductVersion: product.Version, PolicyVersion: policy.Version,
		GuaranteeProfileVersion: guarantee, PassengerRequirement: requirement, Sources: sources, Preferences: preferences,
		CustomerContext: CustomerContextRef{Status: contextStatus, Version: hints.Version}, FrozenAt: now.UTC(),
	}, product, nil
}

// enumNeed turns a saved enumerated preference into a hard need: nil stays
// "not stated", the explicit "none" value is false, anything else is true.
func enumNeed(value *string, none string) *bool {
	if value == nil {
		return nil
	}
	need := !strings.EqualFold(strings.TrimSpace(*value), none) && strings.TrimSpace(*value) != ""
	return &need
}

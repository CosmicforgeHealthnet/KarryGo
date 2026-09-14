// Package logisticscapability is the one contract every logistics authority
// uses to declare what it can actually execute, and the one Vendor Service
// uses to decide whether an authority may be selected for a commerce mission.
//
// It exists because Vendor previously had to encode deep per-service
// knowledge ("small parcels go to Dispatch", "Taxi Goods is single-pickup")
// as its own rules. That is exactly the coupling Section 11 forbids: the only
// service that can truthfully say what Dispatch can carry is Dispatch.
//
// Pricing is deliberately absent. A capability answers "can you physically do
// this", never "what would it cost" — quoting already has its own contract in
// shared/go/commercequote and stays there.
package logisticscapability

import (
	"errors"
	"strings"
	"time"
)

// Path is appended to an authority's own internal commerce base, alongside
// the existing quote and mission paths.
const (
	Path = "/internal/commerce/capabilities"

	// ServiceabilityPath answers whether one concrete request is executable
	// by this authority right now.
	ServiceabilityPath = Path + "/serviceability"
)

// Authority is the canonical logistics authority vocabulary.
//
// These values are the SAME strings shared/go/commercemission.Service already
// puts on the wire and every service already persists. They are restated here
// as one named type so Vendor's internal enums can converge on a single
// vocabulary without a breaking rename of persisted rows or live contracts.
type Authority string

const (
	// AuthorityDispatch is driver-dispatch-delivery-service.
	AuthorityDispatch Authority = "DISPATCH"

	// AuthorityTaxi is driver-taxi-service's GOODS delivery product, never its
	// passenger booking surface.
	//
	// The wire value is "TAXI", not "TAXI_GOODS": that is what every service
	// already persists, signs and allowlists. TaxiGoodsAlias below is accepted
	// on input so a caller using the longer spelling still resolves here.
	AuthorityTaxi Authority = "TAXI"

	// AuthorityHauling is driver-hauling-service, which owns both van and
	// truck classes.
	AuthorityHauling Authority = "HAULING"

	// AuthorityVendorOwn is the vendor's own delivery staff. It is not a
	// platform logistics authority: it creates no mission in any driver
	// service and has no capability endpoint.
	AuthorityVendorOwn Authority = "VENDOR_OWN_DELIVERY"
)

// TaxiGoodsAlias is the longer spelling of AuthorityTaxi accepted on input.
const TaxiGoodsAlias = "TAXI_GOODS"

// ErrUnknownAuthority means the value names no authority this platform has.
var ErrUnknownAuthority = errors.New("logisticscapability: unknown authority")

// ParseAuthority normalizes a caller-supplied authority name.
//
// It accepts both "TAXI" and "TAXI_GOODS" for the Taxi Goods authority, and
// both "VENDOR" and "VENDOR_OWN_DELIVERY" for vendor-own delivery, because
// both spellings of each already exist in persisted Vendor rows. Normalizing
// on read is what lets one canonical type replace the divergent enums without
// migrating historical data.
func ParseAuthority(value string) (Authority, error) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case string(AuthorityDispatch):
		return AuthorityDispatch, nil
	case string(AuthorityTaxi), TaxiGoodsAlias:
		return AuthorityTaxi, nil
	case string(AuthorityHauling):
		return AuthorityHauling, nil
	case string(AuthorityVendorOwn), "VENDOR":
		return AuthorityVendorOwn, nil
	default:
		return "", ErrUnknownAuthority
	}
}

// Valid reports whether the authority is one this platform knows.
func (a Authority) Valid() bool {
	switch a {
	case AuthorityDispatch, AuthorityTaxi, AuthorityHauling, AuthorityVendorOwn:
		return true
	default:
		return false
	}
}

// PlatformExecuted reports whether this authority is a driver service that
// creates a real mission. Vendor-own delivery is not.
func (a Authority) PlatformExecuted() bool {
	switch a {
	case AuthorityDispatch, AuthorityTaxi, AuthorityHauling:
		return true
	default:
		return false
	}
}

// Capability is one authority's truthful, self-declared statement of what it
// can execute.
//
// Every field is answered by the authority itself. Vendor never infers a
// value that is absent here, and never overrides one that is present: an
// authority that under-declares simply is not selected, which is the safe
// direction.
type Capability struct {
	// Authority names the declaring service.
	Authority Authority `json:"authority"`

	// Version increments when the declared capability changes, so a consumer
	// can tell a genuinely changed capability from a re-read of the same one.
	Version int64 `json:"version"`

	// Available reports whether this authority can accept new commerce work at
	// all right now. A deployment with the integration switched off declares
	// false rather than pretending.
	Available bool `json:"available"`

	// UnavailableReason is a stable machine code when Available is false.
	UnavailableReason string `json:"unavailableReason,omitempty"`

	// MultiPickupSupported and MaxPickups describe consolidated pickups. An
	// authority that can only ever collect from one place declares
	// MultiPickupSupported=false and MaxPickups=1.
	MultiPickupSupported bool  `json:"multiPickupSupported"`
	MaxPickups           int64 `json:"maxPickups"`

	// MultiStopSupported describes multiple DROPOFFS, which is a different
	// capability from multiple pickups and is tracked separately.
	MultiStopSupported bool  `json:"multiStopSupported"`
	MaxDropoffs        int64 `json:"maxDropoffs"`

	// SecurePickupSupported means the provider can present a pickup
	// credential the sender verifies.
	SecurePickupSupported bool `json:"securePickupSupported"`

	// SecureDeliverySupported means the receiver's delivery credential is
	// verified by the provider before completion.
	SecureDeliverySupported bool `json:"secureDeliverySupported"`

	// CustodyEvidenceSupported means the authority records an auditable
	// SENDER -> PROVIDER -> RECEIVER custody chain.
	CustodyEvidenceSupported bool `json:"custodyEvidenceSupported"`

	// CorporateProviderSupported means an organization-managed (Fleet)
	// provider can execute this authority's commerce work.
	CorporateProviderSupported bool `json:"corporateProviderSupported"`

	// VehicleClasses are the classes this authority can dispatch, using the
	// same vocabulary as the quote and mission contracts.
	VehicleClasses []string `json:"vehicleClasses,omitempty"`

	// MaxWeightGrams / MaxVolumeMl / MaxLongestDimensionMm / MaxPackageCount
	// bound one mission. Zero means "not declared", NOT "unlimited" -- a
	// consumer must treat an undeclared bound as unknown rather than infinite.
	MaxWeightGrams        int64 `json:"maxWeightGrams,omitempty"`
	MaxVolumeMl           int64 `json:"maxVolumeMl,omitempty"`
	MaxLongestDimensionMm int64 `json:"maxLongestDimensionMm,omitempty"`
	MaxPackageCount       int64 `json:"maxPackageCount,omitempty"`

	// HandlingSupported lists special handling this authority can honour
	// (FRAGILE, CHILLED, FROZEN, ...).
	HandlingSupported []string `json:"handlingSupported,omitempty"`

	// ObservedAt is when the authority computed this answer.
	ObservedAt time.Time `json:"observedAt"`
}

// SupportsHandling reports whether the authority declared this handling code.
func (c Capability) SupportsHandling(code string) bool {
	needle := strings.ToUpper(strings.TrimSpace(code))
	for _, declared := range c.HandlingSupported {
		if strings.ToUpper(strings.TrimSpace(declared)) == needle {
			return true
		}
	}
	return false
}

// SupportsVehicleClass reports whether the authority declared this class.
func (c Capability) SupportsVehicleClass(class string) bool {
	needle := strings.ToUpper(strings.TrimSpace(class))
	if needle == "" {
		return true
	}
	for _, declared := range c.VehicleClasses {
		if strings.ToUpper(strings.TrimSpace(declared)) == needle {
			return true
		}
	}
	return false
}

// Requirement is what one concrete commerce mission needs.
//
// It carries no price, no provider and no vendor commercial data: an
// authority answering serviceability is being asked a physical-execution
// question only.
type Requirement struct {
	PickupCount        int64    `json:"pickupCount"`
	DropoffCount       int64    `json:"dropoffCount"`
	WeightGrams        int64    `json:"weightGrams,omitempty"`
	VolumeMl           int64    `json:"volumeMl,omitempty"`
	LongestDimensionMm int64    `json:"longestDimensionMm,omitempty"`
	PackageCount       int64    `json:"packageCount,omitempty"`
	VehicleClass       string   `json:"vehicleClass,omitempty"`
	Handling           []string `json:"handling,omitempty"`
	SecurePickup       bool     `json:"securePickup,omitempty"`
	SecureDelivery     bool     `json:"secureDelivery,omitempty"`
}

// Reason codes a capability mismatch resolves to. They are stable strings so
// Vendor can surface a specific, actionable refusal instead of a generic one.
const (
	ReasonServiceable           = "SERVICEABLE"
	ReasonAuthorityUnavailable  = "AUTHORITY_UNAVAILABLE"
	ReasonMultiPickupNotAllowed = "MULTI_PICKUP_NOT_SUPPORTED"
	ReasonTooManyPickups        = "TOO_MANY_PICKUPS"
	ReasonMultiStopNotAllowed   = "MULTI_STOP_NOT_SUPPORTED"
	ReasonTooManyDropoffs       = "TOO_MANY_DROPOFFS"
	ReasonWeightExceeded        = "WEIGHT_EXCEEDED"
	ReasonVolumeExceeded        = "VOLUME_EXCEEDED"
	ReasonDimensionExceeded     = "DIMENSION_EXCEEDED"
	ReasonPackageCountExceeded  = "PACKAGE_COUNT_EXCEEDED"
	ReasonVehicleUnsupported    = "VEHICLE_CLASS_UNSUPPORTED"
	ReasonHandlingUnsupported   = "HANDLING_UNSUPPORTED"
	ReasonSecurePickupMissing   = "SECURE_PICKUP_UNSUPPORTED"
	ReasonSecureDeliveryMissing = "SECURE_DELIVERY_UNSUPPORTED"
)

// Match decides whether a capability can carry a requirement.
//
// This is the ONE place the decision is made, shared by the authority (when
// answering serviceability) and by Vendor (when choosing between authorities),
// so the two can never disagree about what a declared capability means.
//
// An undeclared bound (zero) is skipped rather than treated as zero-capacity:
// a bound the authority did not state is unknown, and refusing on it would
// make every authority that omits one look unserviceable.
func Match(capability Capability, requirement Requirement) (bool, string) {
	if !capability.Available {
		return false, ReasonAuthorityUnavailable
	}

	pickups := requirement.PickupCount
	if pickups < 1 {
		pickups = 1
	}
	if pickups > 1 && !capability.MultiPickupSupported {
		return false, ReasonMultiPickupNotAllowed
	}
	if capability.MaxPickups > 0 && pickups > capability.MaxPickups {
		return false, ReasonTooManyPickups
	}

	dropoffs := requirement.DropoffCount
	if dropoffs < 1 {
		dropoffs = 1
	}
	if dropoffs > 1 && !capability.MultiStopSupported {
		return false, ReasonMultiStopNotAllowed
	}
	if capability.MaxDropoffs > 0 && dropoffs > capability.MaxDropoffs {
		return false, ReasonTooManyDropoffs
	}

	if capability.MaxWeightGrams > 0 && requirement.WeightGrams > capability.MaxWeightGrams {
		return false, ReasonWeightExceeded
	}
	if capability.MaxVolumeMl > 0 && requirement.VolumeMl > capability.MaxVolumeMl {
		return false, ReasonVolumeExceeded
	}
	if capability.MaxLongestDimensionMm > 0 && requirement.LongestDimensionMm > capability.MaxLongestDimensionMm {
		return false, ReasonDimensionExceeded
	}
	if capability.MaxPackageCount > 0 && requirement.PackageCount > capability.MaxPackageCount {
		return false, ReasonPackageCountExceeded
	}

	if len(capability.VehicleClasses) > 0 && !capability.SupportsVehicleClass(requirement.VehicleClass) {
		return false, ReasonVehicleUnsupported
	}
	for _, handling := range requirement.Handling {
		if strings.TrimSpace(handling) == "" {
			continue
		}
		if len(capability.HandlingSupported) > 0 && !capability.SupportsHandling(handling) {
			return false, ReasonHandlingUnsupported
		}
	}

	// A protected handover must never silently downgrade to an insecure
	// completion: an authority that cannot verify it is simply not selected.
	if requirement.SecurePickup && !capability.SecurePickupSupported {
		return false, ReasonSecurePickupMissing
	}
	if requirement.SecureDelivery && !capability.SecureDeliverySupported {
		return false, ReasonSecureDeliveryMissing
	}

	return true, ReasonServiceable
}

// Serviceability is the answer to one concrete executability question.
type Serviceability struct {
	Authority    Authority   `json:"authority"`
	Serviceable  bool        `json:"serviceable"`
	ReasonCode   string      `json:"reasonCode"`
	Capabilities *Capability `json:"capabilities,omitempty"`
	Version      int64       `json:"version"`
}

// Evaluate answers serviceability from a declared capability.
func Evaluate(capability Capability, requirement Requirement) Serviceability {
	serviceable, reason := Match(capability, requirement)
	snapshot := capability
	return Serviceability{
		Authority:    capability.Authority,
		Serviceable:  serviceable,
		ReasonCode:   reason,
		Capabilities: &snapshot,
		Version:      capability.Version,
	}
}

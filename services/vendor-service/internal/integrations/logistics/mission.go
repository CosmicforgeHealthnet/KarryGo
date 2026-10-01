package logistics

// The conceptual commerce fulfilment mission (VND-010, VND-012, VND-025).
//
// Each authority keeps its own physical request schema — Dispatch's plan and
// stop model, the shared commerce mission contract for Taxi and Hauling — and
// this does not replace any of them. Forcing three execution services into one
// wire schema would make every future capability difference a schema
// negotiation.
//
// What it does give is one vocabulary for the things commerce must be able to
// state regardless of who executes: which order this is, which authority was
// chosen, where it is collected and delivered, what is being carried, whether
// either handover must be protected, and the two identities that make a retry
// safe and an order traceable end to end.

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// Location is a pickup or delivery point in the conceptual model.
type Location struct {
	Ref       string  `json:"ref,omitempty"`
	Address   string  `json:"address"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// CargoMetadata is the physical description commerce can state for any
// authority. It is what the capability gate is evaluated against.
type CargoMetadata struct {
	WeightGrams        int64    `json:"weight_grams,omitempty"`
	VolumeMl           int64    `json:"volume_ml,omitempty"`
	LongestDimensionMm int64    `json:"longest_dimension_mm,omitempty"`
	PackageCount       int64    `json:"package_count,omitempty"`
	VehicleClass       string   `json:"vehicle_class,omitempty"`
	Handling           []string `json:"handling,omitempty"`
}

// MissionIntent is what Vendor decided, before any authority was contacted.
//
// It names an authority and never a provider. There is no field here for a
// driver, rider, vehicle or fleet member, and that absence is enforced by a
// test rather than left to review (VND-003): provider identity is resolved by
// the execution service or by Fleet, after Vendor's work is done.
type MissionIntent struct {
	// CommerceOrderRef is the parent order this mission fulfils.
	CommerceOrderRef string `json:"commerce_order_ref"`

	// MissionRef is Vendor's own mission identity.
	MissionRef string `json:"mission_ref"`

	// Authority is the execution service chosen at checkout.
	Authority Authority `json:"authority"`

	// SelectedBy records whether this was the frozen checkout decision or an
	// explicit replan, so a later authority change is always deliberate
	// (VND-004).
	SelectedBy SelectionSource `json:"selected_by"`

	// SourceVersion is the version of the plan this intent was cut from. It
	// participates in the idempotency identity so a replan produces a
	// genuinely new mission rather than colliding with the old one.
	SourceVersion int64 `json:"source_version"`

	Pickups  []Location `json:"pickups"`
	Delivery Location   `json:"delivery"`

	Cargo CargoMetadata `json:"cargo"`

	// SecurePickupRequired means the provider must present a pickup
	// credential that the vendor verifies (VND-015). Commerce states the
	// requirement; the authority owns the credential.
	SecurePickupRequired bool `json:"secure_pickup_required"`

	// SecureDeliveryRequired means the receiver's delivery credential must be
	// verified by the executing provider (VND-016). A protected handover is
	// never allowed to complete without it.
	SecureDeliveryRequired bool `json:"secure_delivery_required"`

	// CorrelationID threads one order across Vendor, the execution service
	// and Fleet (VND-025).
	CorrelationID string `json:"correlation_id"`
}

// ErrInvalidIntent means the intent cannot be executed as stated.
var ErrInvalidIntent = errors.New("logistics: invalid mission intent")

// Validate checks the intent is internally coherent.
//
// It deliberately does not check serviceability: that is the authority's
// answer, obtained through the registry, and conflating "Vendor wrote this
// down wrong" with "the authority will not do it" would make an outage look
// like a bug.
func (m MissionIntent) Validate() error {
	switch {
	case strings.TrimSpace(m.CommerceOrderRef) == "":
		return fmt.Errorf("%w: commerce order reference is required", ErrInvalidIntent)
	case strings.TrimSpace(m.MissionRef) == "":
		return fmt.Errorf("%w: mission reference is required", ErrInvalidIntent)
	case !m.Authority.PlatformExecuted():
		return fmt.Errorf("%w: %q is not a platform execution authority", ErrInvalidIntent, m.Authority)
	case !m.SelectedBy.Valid():
		return fmt.Errorf("%w: authority selection source is required", ErrInvalidIntent)
	case len(m.Pickups) == 0:
		return fmt.Errorf("%w: at least one pickup is required", ErrInvalidIntent)
	case strings.TrimSpace(m.Delivery.Address) == "":
		return fmt.Errorf("%w: a delivery location is required", ErrInvalidIntent)
	case strings.TrimSpace(m.CorrelationID) == "":
		return fmt.Errorf("%w: a correlation id is required", ErrInvalidIntent)
	}
	return nil
}

// Requirement projects the intent onto the capability contract.
//
// Pickup count comes from the intent's own pickups rather than a declared
// number, so a three-pickup order cannot be described to the gate as a
// one-pickup order (VND-030).
func (m MissionIntent) Requirement() Requirement {
	return Requirement{
		PickupCount:        int64(len(m.Pickups)),
		DropoffCount:       1,
		WeightGrams:        m.Cargo.WeightGrams,
		VolumeMl:           m.Cargo.VolumeMl,
		LongestDimensionMm: m.Cargo.LongestDimensionMm,
		PackageCount:       m.Cargo.PackageCount,
		VehicleClass:       m.Cargo.VehicleClass,
		Handling:           m.Cargo.Handling,
		SecurePickup:       m.SecurePickupRequired,
		SecureDelivery:     m.SecureDeliveryRequired,
	}
}

// IdempotencyKey is the deterministic execution identity (VND-012).
//
// Vendor mission ref, authority and source version, exactly as the spec
// requires. Two retries of the same intent produce the same key and therefore
// the same execution; a replan changes the source version and therefore
// produces a different one, which is the only way an authority may legitimately
// end up holding two executions for one order.
func (m MissionIntent) IdempotencyKey() string {
	return fmt.Sprintf("commerce-mission:%s:%s:%d",
		strings.TrimSpace(m.MissionRef), m.Authority, m.SourceVersion)
}

// Fingerprint is a stable digest of the executable content of the intent.
//
// It answers a different question from the idempotency key: the key says "is
// this the same mission?", the fingerprint says "is this the same mission
// asking for the same thing?". A retry whose fingerprint has changed is a
// conflict, not a duplicate, and must not be sent under the original key.
func (m MissionIntent) Fingerprint() string {
	builder := &strings.Builder{}
	fmt.Fprintf(builder, "%s|%s|%s|%d|", m.CommerceOrderRef, m.MissionRef, m.Authority, m.SourceVersion)
	for _, pickup := range m.Pickups {
		fmt.Fprintf(builder, "P:%s:%s:%.6f:%.6f|", pickup.Ref, pickup.Address, pickup.Latitude, pickup.Longitude)
	}
	fmt.Fprintf(builder, "D:%s:%s:%.6f:%.6f|", m.Delivery.Ref, m.Delivery.Address, m.Delivery.Latitude, m.Delivery.Longitude)
	fmt.Fprintf(builder, "C:%d:%d:%d:%d:%s:%s|", m.Cargo.WeightGrams, m.Cargo.VolumeMl,
		m.Cargo.LongestDimensionMm, m.Cargo.PackageCount, m.Cargo.VehicleClass,
		strings.Join(m.Cargo.Handling, ","))
	fmt.Fprintf(builder, "S:%t:%t", m.SecurePickupRequired, m.SecureDeliveryRequired)
	sum := sha256.Sum256([]byte(builder.String()))
	return hex.EncodeToString(sum[:])
}

// ProviderSelectionFields are the field names a commerce mission request must
// never carry (VND-003).
//
// Naming them here, rather than only in a test, is deliberate: a reviewer
// adding a "preferred rider" field sees the rule next to the model instead of
// discovering it in a failing test they do not recognise.
func ProviderSelectionFields() []string {
	return []string{
		"provider", "providerid", "provider_id", "providerref", "provider_ref",
		"driver", "driverid", "driver_id",
		"rider", "riderid", "rider_id",
		"vehicle_id", "vehicleid",
		"fleetmember", "fleet_member", "fleetmemberid", "fleet_member_id",
		"assignee", "assigneeid", "assignee_id",
		"preferredprovider", "preferred_provider",
	}
}

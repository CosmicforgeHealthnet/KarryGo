// Package goods owns the distinct Vendor Commerce Taxi Goods aggregate.
// Nothing in this package creates or mutates passenger taxi_bookings.
package goods

import (
	"errors"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/commercequote"
	"cosmicforge/logistics/shared/go/custodychain"
)

const (
	CapabilityApproved    = "APPROVED"
	CapabilityNotApproved = "NOT_APPROVED"
	CapabilitySuspended   = "SUSPENDED"
)

const (
	StatusMatching                 = "MATCHING"
	StatusOffered                  = "OFFERED"
	StatusAssigned                 = "ASSIGNED"
	StatusEnRoutePickup            = "EN_ROUTE_PICKUP"
	StatusAtPickup                 = "AT_PICKUP"
	StatusPickupHandoverRequired   = "PICKUP_HANDOVER_REQUIRED"
	StatusPickedUp                 = "PICKED_UP"
	StatusInTransit                = "IN_TRANSIT"
	StatusAtDropoff                = "AT_DROPOFF"
	StatusDeliveryHandoverRequired = "DELIVERY_HANDOVER_REQUIRED"
	StatusDelivered                = "DELIVERED"
	StatusCompleted                = "COMPLETED"
	StatusCancelled                = "CANCELLED"
	StatusUnmatched                = "UNMATCHED"
	StatusFailed                   = "FAILED"
)

const (
	StagePickup   = "PICKUP"
	StageDelivery = "DELIVERY"
)

var (
	ErrDisabled           = errors.New("taxi goods delivery is disabled")
	ErrInvalid            = errors.New("taxi goods request is invalid")
	ErrNotFound           = errors.New("taxi goods resource not found")
	ErrConflict           = errors.New("taxi goods state conflict")
	ErrUnavailable        = errors.New("taxi goods dependency unavailable")
	ErrCredentialInvalid  = errors.New("handover credential is invalid")
	ErrCredentialExpired  = errors.New("handover credential is expired")
	ErrCredentialLocked   = errors.New("handover credential is locked")
	ErrResendUnavailable  = errors.New("handover resend is unavailable")
	ErrEvidenceRequired   = errors.New("required evidence is missing")
	ErrMediaScopeMismatch = errors.New("media reference is outside mission scope")
)

type ProviderSettings struct {
	ProviderID       string `json:"provider_id"`
	CapabilityStatus string `json:"capability_status"`
	GoodsAvailable   bool   `json:"goods_available"`
	Version          int64  `json:"version"`
}

type CargoProfile struct {
	VehicleID            string   `json:"vehicle_id"`
	MaxWeightGrams       int64    `json:"max_weight_grams"`
	MaxVolumeML          int64    `json:"max_volume_ml"`
	MaxPackageCount      int      `json:"max_package_count"`
	LongestDimensionMM   int      `json:"longest_dimension_mm"`
	HandlingCapabilities []string `json:"handling_capabilities"`
	Verified             bool     `json:"verified"`
	Version              int64    `json:"version"`
}

func (p CargoProfile) Validate() error {
	if strings.TrimSpace(p.VehicleID) == "" || p.MaxWeightGrams < 0 || p.MaxWeightGrams > 1_000_000 ||
		p.MaxVolumeML < 0 || p.MaxVolumeML > 10_000_000 || p.MaxPackageCount < 0 ||
		p.MaxPackageCount > 100 || p.LongestDimensionMM < 0 || p.LongestDimensionMM > 5_000 {
		return ErrInvalid
	}
	allowed := map[string]bool{
		commercequote.CapabilityFragileCarry: true, commercequote.CapabilityUprightCarry: true,
		commercequote.CapabilityLeakContainment: true, commercequote.CapabilityInsulated: true,
		commercequote.CapabilityHotHold: true, commercequote.CapabilityColdChain: true,
		commercequote.CapabilityFrozenChain: true,
	}
	seen := map[string]bool{}
	for _, capability := range p.HandlingCapabilities {
		capability = strings.ToUpper(strings.TrimSpace(capability))
		if !allowed[capability] || seen[capability] {
			return ErrInvalid
		}
		seen[capability] = true
	}
	return nil
}

type Mission struct {
	ID                       string              `json:"external_ref"`
	MissionRef               string              `json:"mission_ref"`
	OperationRef             string              `json:"operation_ref"`
	ParentOrderRef           string              `json:"parent_order_ref,omitempty"`
	CustomerRef              string              `json:"customer_ref,omitempty"`
	ReceiverRef              string              `json:"-"`
	VendorOrderRefs          []string            `json:"vendor_order_refs,omitempty"`
	ProviderID               string              `json:"provider_id,omitempty"`
	VehicleID                string              `json:"vehicle_id,omitempty"`
	Status                   string              `json:"status"`
	Version                  int64               `json:"version"`
	AgreedFeeMinor           int64               `json:"accepted_fee_minor"`
	Currency                 string              `json:"currency"`
	Pickup                   commercequote.Point `json:"pickup"`
	Dropoff                  commercequote.Point `json:"dropoff"`
	ParcelCount              int                 `json:"parcel_count"`
	ManifestRef              string              `json:"manifest_ref,omitempty"`
	ManifestVersion          int64               `json:"manifest_version,omitempty"`
	PickupHandoverStatus     string              `json:"pickup_handover_status,omitempty"`
	DeliveryHandoverStatus   string              `json:"delivery_handover_status,omitempty"`
	PickupEvidenceRequired   bool                `json:"pickup_evidence_required"`
	DeliveryEvidenceRequired bool                `json:"delivery_evidence_required"`
	PickupEvidenceRef        string              `json:"pickup_evidence_ref,omitempty"`
	DeliveryEvidenceRef      string              `json:"delivery_evidence_ref,omitempty"`
	SettlementStatus         string              `json:"settlement_status,omitempty"`
	RequestedAt              time.Time           `json:"requested_at"`
	CreatedAt                time.Time           `json:"created_at"`
}

// ProviderMission is the operational projection returned to Taxi providers.
// Commerce pricing, settlement state, customer identity, and Vendor order
// references remain on the internal authority model and are never serialized
// through provider routes.
type ProviderMission struct {
	MissionRef               string              `json:"mission_ref"`
	Status                   string              `json:"status"`
	Version                  int64               `json:"version"`
	Pickup                   commercequote.Point `json:"pickup"`
	Dropoff                  commercequote.Point `json:"dropoff"`
	ParcelCount              int                 `json:"parcel_count"`
	ManifestRef              string              `json:"manifest_ref,omitempty"`
	ManifestVersion          int64               `json:"manifest_version,omitempty"`
	PickupHandoverStatus     string              `json:"pickup_handover_status,omitempty"`
	DeliveryHandoverStatus   string              `json:"delivery_handover_status,omitempty"`
	PickupEvidenceRequired   bool                `json:"pickup_evidence_required"`
	DeliveryEvidenceRequired bool                `json:"delivery_evidence_required"`
	RequestedAt              time.Time           `json:"requested_at"`
}

func providerMission(m Mission) ProviderMission {
	return ProviderMission{
		MissionRef: m.MissionRef, Status: m.Status, Version: m.Version,
		Pickup: m.Pickup, Dropoff: m.Dropoff, ParcelCount: m.ParcelCount,
		ManifestRef: m.ManifestRef, ManifestVersion: m.ManifestVersion,
		PickupHandoverStatus:     m.PickupHandoverStatus,
		DeliveryHandoverStatus:   m.DeliveryHandoverStatus,
		PickupEvidenceRequired:   m.PickupEvidenceRequired,
		DeliveryEvidenceRequired: m.DeliveryEvidenceRequired,
		RequestedAt:              m.RequestedAt,
	}
}

type Credential struct {
	ChallengeRef string    `json:"challenge_ref"`
	Stage        string    `json:"stage"`
	OTP          string    `json:"otp,omitempty"`
	QRToken      string    `json:"qr_token,omitempty"`
	Version      int64     `json:"version"`
	ExpiresAt    time.Time `json:"expires_at"`

	// Direction states who holds this credential and who may verify it.
	// Taxi Goods runs both directions on one type — the pickup credential is
	// provider-held and vendor-verified, the delivery credential is
	// receiver-held and provider-verified — so a consumer reading only Stage
	// would have to know that mapping itself.
	Direction custodychain.Direction `json:"direction"`
}

// PurposeForStage maps a Taxi Goods handover stage onto its credential
// purpose.
//
// Taxi Goods missions are Commerce work by definition: they are created by
// Vendor Service through the internal commerce-mission contract, and the
// sender is always a merchant. Direct Taxi pickup, where the passenger holds
// the credential instead, is a different feature on a different surface and
// deliberately does not resolve here.
func PurposeForStage(stage string) (custodychain.CredentialPurpose, bool) {
	switch strings.ToUpper(strings.TrimSpace(stage)) {
	case StagePickup:
		return custodychain.PurposeCommercePickup, true
	case StageDelivery:
		return custodychain.PurposeDelivery, true
	default:
		return "", false
	}
}

// WithDirection stamps the canonical direction for the credential's stage.
func (c Credential) WithDirection(missionRef string) Credential {
	purpose, ok := PurposeForStage(c.Stage)
	if !ok {
		return c
	}
	expires := c.ExpiresAt
	if direction, built := custodychain.DirectionFor(
		purpose, "TAXI", missionRef, &expires); built {
		c.Direction = direction
	}
	return c
}

type VerifyCredential struct {
	ChallengeRef string `json:"challenge_ref"`
	Method       string `json:"method"`
	Credential   string `json:"credential"`
	OperationRef string `json:"operation_ref"`
	ReceiverRef  string `json:"receiver_ref,omitempty"`
}

type EvidenceCommand struct {
	Stage      string    `json:"stage"`
	MediaRef   string    `json:"media_ref"`
	ParcelRefs []string  `json:"parcel_refs"`
	CapturedAt time.Time `json:"captured_at"`
}

// ReassignProviderCommand is an operations-only recovery command. Provider
// matching remains Taxi-owned; Vendor Commerce cannot choose this identity.
type ReassignProviderCommand struct {
	NewProviderID   string `json:"new_provider_id"`
	NewVehicleID    string `json:"new_vehicle_id"`
	OperationRef    string `json:"operation_ref"`
	ExpectedVersion int64  `json:"expected_version"`
}

// ReceiverChangeCommand rotates the delivery authority binding before final
// custody. Contact details are stored on the mission but never emitted to the
// provider projection or lifecycle outbox.
type ReceiverChangeCommand struct {
	ReceiverRef     string `json:"receiver_ref"`
	ReceiverName    string `json:"receiver_name"`
	ReceiverPhone   string `json:"receiver_phone"`
	OperationRef    string `json:"operation_ref"`
	ExpectedVersion int64  `json:"expected_version"`
}

type HandoverRecord struct {
	ChallengeRef      string
	MissionID         string
	Stage             string
	ProviderID        string
	ReceiverRef       string
	OTPHash           string
	QRHash            string
	Version           int64
	Status            string
	Attempts          int
	ResendCount       int
	ExpiresAt         time.Time
	ResendAvailableAt time.Time
}

// Package booking owns the customer-facing dispatch delivery booking:
// persistence of the booking record, fare estimation, wallet escrow, and the
// customer status read model derived from the request broadcast + trip rows.
package booking

import (
	"cosmicforge/logistics/shared/go/journeyshare"
	"time"

	"cosmicforge/logistics/shared/go/dispatchpricing"
)

// Coarse persisted statuses on dispatch_bookings.status. The fine-grained
// customer-facing status is always derived from the broadcast + trip rows.
const (
	StatusSearching = "searching"
	StatusAccepted  = "accepted"
	StatusCompleted = "completed"
	StatusCancelled = "cancelled"
	StatusUnmatched = "unmatched"
)

// Wire statuses returned to the customer app.
const (
	WireSearching       = "searching"
	WireUnmatched       = "unmatched"
	WireCancelled       = "cancelled"
	WireAccepted        = "accepted"
	WireEnRoutePickup   = "en_route_pickup"
	WireArrivedAtPickup = "arrived_at_pickup"
	WireInTransit       = "in_transit"
	WireDelivered       = "delivered"
	WireCompleted       = "completed"
)

const (
	PaymentUnpaid   = "unpaid"
	PaymentHeld     = "held"
	PaymentPaid     = "paid"
	PaymentRefunded = "refunded"
	PaymentFailed   = "failed"
)

const (
	PackageSizeSmall  = "small"
	PackageSizeMedium = "medium"
	PackageSizeLarge  = "large"

	ServiceTierStandard = "standard"
	ServiceTierExpress  = "express"

	PaymentMethodWallet = "wallet"
)

type Booking struct {
	ID                         string   `json:"id"`
	CustomerID                 string   `json:"customer_id"`
	Status                     string   `json:"-"` // coarse persisted status; wire status is derived
	PickupAddress              string   `json:"pickup_address"`
	PickupLat                  float64  `json:"pickup_lat"`
	PickupLng                  float64  `json:"pickup_lng"`
	DropoffAddress             string   `json:"dropoff_address"`
	DropoffLat                 float64  `json:"dropoff_lat"`
	DropoffLng                 float64  `json:"dropoff_lng"`
	DistanceKM                 float64  `json:"distance_km"`
	FareAmount                 int64    `json:"fare_amount_kobo"`
	Currency                   string   `json:"currency"`
	PaymentMethod              string   `json:"payment_method"`
	PaymentStatus              string   `json:"payment_status"`
	PaymentRef                 string   `json:"-"`
	ReceiverName               string   `json:"receiver_name"`
	ReceiverPhone              string   `json:"receiver_phone"`
	ReceiverEmail              string   `json:"-"`
	ReceiverCustomerID         string   `json:"-"`
	PackageDesc                string   `json:"package_desc"`
	PackageWeight              *float64 `json:"package_weight,omitempty"`
	PackageType                string   `json:"package_type"`
	PackageSize                string   `json:"package_size"`
	IsFragile                  bool     `json:"is_fragile"`
	ServiceTier                string   `json:"service_tier"`
	ServiceabilityQuoteRef     string   `json:"serviceability_quote_ref,omitempty"`
	ServiceabilityQuoteVersion int64    `json:"serviceability_quote_version,omitempty"`

	RolloutProtectionMode    string `json:"rollout_protection_mode,omitempty"`
	RolloutPolicyVersion     string `json:"rollout_policy_version,omitempty"`
	HandoverRequired         bool   `json:"handover_required,omitempty"`
	PickupEvidenceRequired   bool   `json:"pickup_evidence_required,omitempty"`
	DeliveryEvidenceRequired bool   `json:"delivery_evidence_required,omitempty"`
	ProviderTransferRequired bool   `json:"provider_transfer_required,omitempty"`

	ProviderID string `json:"provider_id,omitempty"`

	// Route geometry for the customer's map. The pickup leg only appears
	// once a rider is assigned; both are absent when no routing API key is
	// configured and the app falls back to a straight line.
	RoutePickupPolyline   string `json:"route_pickup_polyline,omitempty"`
	RoutePickupDistanceM  *int64 `json:"route_pickup_distance_m,omitempty"`
	RoutePickupDurationS  *int64 `json:"route_pickup_duration_s,omitempty"`
	RouteDropoffPolyline  string `json:"route_dropoff_polyline,omitempty"`
	RouteDropoffDistanceM *int64 `json:"route_dropoff_distance_m,omitempty"`
	RouteDropoffDurationS *int64 `json:"route_dropoff_duration_s,omitempty"`

	CancelReason string     `json:"cancel_reason,omitempty"`
	CancelledAt  *time.Time `json:"cancelled_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

// DerivedState is the joined broadcast + trip state used to compute the
// customer-facing wire status for a booking.
type DerivedState struct {
	Booking         Booking
	BroadcastStatus string // "" when no broadcast row exists yet
	TripID          string
	TripStatus      string // "" when no trip row exists
	TripProviderID  string

	// Delivery handover credential (customer's own code, verified by the
	// rider). Read-only here â€” resend/generation is owned by the trip
	// feature via TripControl. The rider's pickup credential is never
	// exposed to the customer, so it is not part of this state.
	DeliveryCode          string
	DeliveryCodeExpiresAt *time.Time
	DeliveryCodeAttempts  int
}

// WireStatus maps the persisted booking + broadcast + trip state onto the
// customer-facing status. Precedence: trip > booking terminal > broadcast.
func (d DerivedState) WireStatus() string {
	switch d.TripStatus {
	case "assigned":
		return WireAccepted
	case "en_route_pickup":
		return WireEnRoutePickup
	case "arrived_pickup":
		return WireArrivedAtPickup
	case "in_progress":
		return WireInTransit
	case "proof_submitted":
		return WireDelivered
	case "completed":
		return WireCompleted
	case "cancelled", "failed":
		return WireCancelled
	}
	// Unmatched before cancelled: unmatched bookings also carry a cancelled_at
	// timestamp, so the order matters.
	if d.Booking.Status == StatusUnmatched {
		return WireUnmatched
	}
	if d.Booking.CancelledAt != nil || d.Booking.Status == StatusCancelled {
		return WireCancelled
	}
	switch d.BroadcastStatus {
	case "cancelled":
		return WireCancelled
	case "no_provider_found", "expired":
		return WireUnmatched
	case "accepted":
		// Accept landed but the trip subscriber has not created the trip yet.
		return WireAccepted
	}
	return WireSearching
}

// BookingView is the wire representation returned to the customer app.
type BookingView struct {
	Booking
	Status     string                       `json:"status"`
	TripID     string                       `json:"trip_id,omitempty"`
	Operations *BookingOperationsProjection `json:"operations,omitempty"`
}

func (d DerivedState) View() BookingView {
	view := BookingView{Booking: d.Booking, Status: d.WireStatus(), TripID: d.TripID}
	if view.ProviderID == "" && d.TripProviderID != "" {
		view.ProviderID = d.TripProviderID
	}
	return view
}

type LatLngAddress struct {
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	Address string  `json:"address"`
}

type CreateBookingInput struct {
	Pickup                 LatLngAddress `json:"pickup"`
	Dropoff                LatLngAddress `json:"dropoff"`
	ReceiverName           string        `json:"receiver_name"`
	ReceiverPhone          string        `json:"receiver_phone"`
	ReceiverEmail          string        `json:"receiver_email,omitempty"`
	ReceiverCustomerID     string        `json:"receiver_customer_id,omitempty"`
	PackageDesc            string        `json:"package_desc"`
	PackageWeight          float64       `json:"package_weight"`
	PackageType            string        `json:"package_type"`
	PackageSize            string        `json:"package_size"`
	IsFragile              bool          `json:"is_fragile"`
	ServiceTier            string        `json:"service_tier"`
	PaymentMethod          string        `json:"payment_method"`
	ServiceabilityQuoteRef string        `json:"serviceability_quote_ref,omitempty"`
}

// BookingOperationsProjection is an additive, secret-free customer read model
// over the normalized Sections 4-12 tables. Absence means the legacy booking
// has not entered an additive operational capability.
type BookingOperationsProjection struct {
	DeliveryGroup  *DeliveryGroupSummary  `json:"delivery_group,omitempty"`
	Mission        *MissionSummary        `json:"mission,omitempty"`
	Serviceability *ServiceabilitySummary `json:"serviceability,omitempty"`
	Assignment     *AssignmentSummary     `json:"assignment,omitempty"`
	Handover       *HandoverSummary       `json:"handover,omitempty"`
	Evidence       *EvidenceSummary       `json:"evidence,omitempty"`
	Recovery       *RecoverySummary       `json:"recovery,omitempty"`
	Route          *RouteSummary          `json:"route,omitempty"`
}

type DeliveryGroupSummary struct {
	ID      string `json:"id"`
	State   string `json:"state"`
	Version int64  `json:"version"`
}

type MissionSummary struct {
	ID              string `json:"id"`
	State           string `json:"state"`
	Version         int64  `json:"version"`
	ParcelCount     int64  `json:"parcel_count"`
	ManifestRef     string `json:"manifest_ref,omitempty"`
	ManifestVersion int64  `json:"manifest_version,omitempty"`
	ManifestStatus  string `json:"manifest_status,omitempty"`
}

type ServiceabilitySummary struct {
	Reference     string    `json:"reference"`
	Decision      string    `json:"decision"`
	PolicyVersion string    `json:"policy_version"`
	Version       int64     `json:"version"`
	ExpiresAt     time.Time `json:"expires_at"`
}

type AssignmentSummary struct {
	OfferStatus string `json:"offer_status,omitempty"`
	QueueStatus string `json:"queue_status,omitempty"`
}

type HandoverSummary struct {
	Stage  string `json:"stage"`
	Status string `json:"status"`
}

type EvidenceSummary struct {
	Count int64 `json:"count"`
}

type RecoverySummary struct {
	AttemptState string `json:"attempt_state,omitempty"`
	Action       string `json:"action,omitempty"`
	ReturnState  string `json:"return_state,omitempty"`
}

type RouteSummary struct {
	Version        int64  `json:"version"`
	Status         string `json:"status"`
	OpenAlertCount int64  `json:"open_alert_count"`
}

type EstimateRequest struct {
	PickupLat   float64 `json:"pickup_lat"`
	PickupLng   float64 `json:"pickup_lng"`
	DropoffLat  float64 `json:"dropoff_lat"`
	DropoffLng  float64 `json:"dropoff_lng"`
	PackageSize string  `json:"package_size"`
	ServiceTier string  `json:"service_tier"`
}

type FareBreakdown struct {
	BaseFareKobo         int64 `json:"base_fare_kobo"`
	DistanceFareKobo     int64 `json:"distance_fare_kobo"`
	SizeSurchargeKobo    int64 `json:"size_surcharge_kobo"`
	ExpressSurchargeKobo int64 `json:"express_surcharge_kobo"`
}

type EstimateResponse struct {
	DistanceKM       float64       `json:"distance_km"`
	FareEstimateKobo int64         `json:"fare_estimate_kobo"`
	Currency         string        `json:"currency"`
	ETA              string        `json:"eta"`
	Breakdown        FareBreakdown `json:"breakdown"`
}

type AvailabilityResponse struct {
	Available bool  `json:"available"`
	Count     int64 `json:"count"`
}

// LocationResponse is the rider's live position for the customer's trip map.
//
// Heading, speed and accuracy have always been collected by the rider app and
// stored, but were dropped here, so the customer's marker could not be rotated
// or dead-reckoned between fixes. They are carried through now.
//
// The point is bound to the trip's current assignment (never the rider's
// global position) and State is FRESH or STALE by server policy. The rider's
// internal id is deliberately not part of the customer response.
type LocationResponse struct {
	Lat       float64   `json:"lat"`
	Lng       float64   `json:"lng"`
	Heading   float64   `json:"heading"`
	Speed     float64   `json:"speed"`
	Accuracy  float64   `json:"accuracy"`
	UpdatedAt time.Time `json:"updated_at"`
	State     string    `json:"state"`
	// When the rider should reach the pickup and the drop-off; absent when it
	// cannot be estimated.
	journeyshare.ETAs
}

type ReviewInput struct {
	Rating             int    `json:"rating"`
	ReviewText         string `json:"review_text"`
	RecommendsProvider *bool  `json:"recommends_provider"`
}

type Review struct {
	ID                 string    `json:"id"`
	BookingID          string    `json:"booking_id"`
	ProviderID         string    `json:"provider_id"`
	CustomerID         string    `json:"customer_id"`
	Rating             int       `json:"rating"`
	ReviewText         string    `json:"review_text,omitempty"`
	RecommendsProvider *bool     `json:"recommends_provider,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}

type ListBookingsOptions struct {
	Limit  int
	Offset int
}

func NormalizePackageSize(size string) string {
	return dispatchpricing.NormalizeSize(size)
}

func NormalizeServiceTier(tier string) string {
	return dispatchpricing.NormalizeTier(tier)
}

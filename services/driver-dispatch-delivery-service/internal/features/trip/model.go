package trip

import (
	"fmt"
	"net/http"
	"time"

	"karrygo/shared/go/apperrors"
)

type TripStatus string

const (
	StatusAssigned       TripStatus = "assigned"
	StatusEnRoutePickup  TripStatus = "en_route_pickup"
	StatusArrivedPickup  TripStatus = "arrived_pickup"
	StatusInProgress     TripStatus = "in_progress"
	StatusProofSubmitted TripStatus = "proof_submitted"
	StatusCompleted      TripStatus = "completed"
	StatusCancelled      TripStatus = "cancelled"
	StatusFailed         TripStatus = "failed"
)

type CancelledBy string

const (
	CancelledByProvider CancelledBy = "provider"
	CancelledByCustomer CancelledBy = "customer"
	CancelledBySystem   CancelledBy = "system"
	CancelledByAdmin    CancelledBy = "admin"
)

var ValidCancelReasons = map[string]struct{}{
	"rider_unavailable":    {},
	"package_not_ready":    {},
	"wrong_address":        {},
	"customer_unreachable": {},
	"safety_concern":       {},
	"other":                {},
}

var validTransitions = map[TripStatus][]TripStatus{
	StatusAssigned:       {StatusEnRoutePickup, StatusArrivedPickup, StatusCancelled, StatusFailed},
	StatusEnRoutePickup:  {StatusArrivedPickup, StatusCancelled, StatusFailed},
	StatusArrivedPickup:  {StatusInProgress, StatusCancelled, StatusFailed},
	StatusInProgress:     {StatusProofSubmitted, StatusCancelled, StatusFailed},
	StatusProofSubmitted: {StatusCompleted, StatusFailed},
	StatusCompleted:      {},
	StatusCancelled:      {},
	StatusFailed:         {},
}

func IsValidTripStatus(status TripStatus) bool {
	_, ok := validTransitions[status]
	return ok
}

func CanTransition(from, to TripStatus) bool {
	for _, allowed := range validTransitions[from] {
		if allowed == to {
			return true
		}
	}
	return false
}

func ValidateTransition(from, to TripStatus) error {
	if CanTransition(from, to) {
		return nil
	}
	err := apperrors.New(
		http.StatusConflict,
		"invalid_trip_transition",
		fmt.Sprintf("Cannot transition trip from %s to %s.", from, to),
		nil,
	)
	err.Details = map[string]interface{}{
		"from_status": from,
		"to_status":   to,
	}
	return err
}

func CanProviderCancel(status TripStatus) bool {
	switch status {
	case StatusAssigned, StatusEnRoutePickup, StatusArrivedPickup, StatusInProgress:
		return true
	default:
		return false
	}
}

type Trip struct {
	ID             string     `json:"id"`
	BookingID      string     `json:"booking_id"`
	ProviderID     string     `json:"provider_id"`
	CustomerID     string     `json:"customer_id"`
	Status         TripStatus `json:"status"`
	PickupAddress  string     `json:"pickup_address"`
	PickupLat      float64    `json:"pickup_lat"`
	PickupLng      float64    `json:"pickup_lng"`
	DropoffAddress string     `json:"dropoff_address"`
	DropoffLat     float64    `json:"dropoff_lat"`
	DropoffLng     float64    `json:"dropoff_lng"`
	DistanceKM     float64    `json:"distance_km"`
	FareAmount     int64      `json:"fare_amount"`
	Currency       string     `json:"currency"`
	ReceiverName   string     `json:"receiver_name"`
	ReceiverPhone  string     `json:"receiver_phone"`
	PackageDesc    *string    `json:"package_desc,omitempty"`
	PackageWeight  *float64   `json:"package_weight,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	ArrivedAt      *time.Time `json:"arrived_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	CancelledAt    *time.Time `json:"cancelled_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type TripStateLog struct {
	ID         string      `json:"id"`
	TripID     string      `json:"trip_id"`
	FromStatus string      `json:"from_status"`
	ToStatus   TripStatus  `json:"to_status"`
	ChangedAt  time.Time   `json:"changed_at"`
	ChangedBy  CancelledBy `json:"changed_by"`
	Notes      *string     `json:"notes,omitempty"`
}

type DeliveryProof struct {
	ID            string     `json:"id"`
	TripID        string     `json:"trip_id"`
	PhotoRef      string     `json:"photo_ref"`
	SignatureRef  string     `json:"signature_ref"`
	ReceiverName  string     `json:"receiver_name"`
	ReceiverPhone string     `json:"receiver_phone"`
	SubmittedAt   time.Time  `json:"submitted_at"`
	Verified      bool       `json:"verified"`
	VerifiedAt    *time.Time `json:"verified_at,omitempty"`
}

type Cancellation struct {
	ID             string      `json:"id"`
	TripID         string      `json:"trip_id"`
	CancelledBy    CancelledBy `json:"cancelled_by"`
	ReasonCode     string      `json:"reason_code"`
	ReasonText     *string     `json:"reason_text,omitempty"`
	PenaltyApplied bool        `json:"penalty_applied"`
	CancelledAt    time.Time   `json:"cancelled_at"`
}

type CreateTripInput struct {
	BookingID      string
	ProviderID     string
	CustomerID     string
	PickupAddress  string
	PickupLat      float64
	PickupLng      float64
	DropoffAddress string
	DropoffLat     float64
	DropoffLng     float64
	DistanceKM     float64
	FareAmount     int64
	Currency       string
	ReceiverName   string
	ReceiverPhone  string
	PackageDesc    string
	PackageWeight  float64
}

type StateLogInput struct {
	TripID     string
	FromStatus string
	ToStatus   TripStatus
	ChangedBy  CancelledBy
	Notes      string
}

type CreateProofInput struct {
	TripID        string
	PhotoRef      string
	SignatureRef  string
	ReceiverName  string
	ReceiverPhone string
}

type CreateCancellationInput struct {
	TripID      string
	CancelledBy CancelledBy
	ReasonCode  string
	ReasonText  string
}

type ListTripsOptions struct {
	Status TripStatus
	Limit  int
	Offset int
}

type TripListItem struct {
	ID             string     `json:"id"`
	BookingID      string     `json:"booking_id"`
	Status         TripStatus `json:"status"`
	PickupAddress  string     `json:"pickup_address"`
	DropoffAddress string     `json:"dropoff_address"`
	DistanceKM     float64    `json:"distance_km"`
	FareAmount     int64      `json:"fare_amount"`
	Currency       string     `json:"currency"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
}

type TripListResponse struct {
	Trips []TripListItem `json:"trips"`
	Total int            `json:"total"`
	Page  int            `json:"page"`
	Limit int            `json:"limit"`
}

type TripDetailResponse struct {
	Trip
	StateLog []TripStateLog `json:"state_log"`
	Proof    *DeliveryProof `json:"proof"`
}

type TripResponse = TripDetailResponse
type ActiveTripResponse = Trip

type TransitionTripInput struct {
	TripID     string
	FromStatus TripStatus
	ToStatus   TripStatus
	ChangedBy  CancelledBy
	Notes      string
	ChangedAt  time.Time
}

type RequestAcceptedEvent struct {
	Event          string    `json:"event"`
	CorrelationID  string    `json:"correlation_id,omitempty"`
	BookingID      string    `json:"booking_id"`
	BroadcastID    string    `json:"broadcast_id"`
	InboxID        string    `json:"inbox_id"`
	ProviderID     string    `json:"provider_id"`
	CustomerID     string    `json:"customer_id,omitempty"`
	FareAmount     int64     `json:"fare_amount"`
	Currency       string    `json:"currency"`
	PickupLat      float64   `json:"pickup_lat"`
	PickupLng      float64   `json:"pickup_lng"`
	PickupAddress  string    `json:"pickup_address"`
	DropoffLat     float64   `json:"dropoff_lat"`
	DropoffLng     float64   `json:"dropoff_lng"`
	DropoffAddress string    `json:"dropoff_address"`
	DistanceKM     float64   `json:"distance_km"`
	ReceiverName   string    `json:"receiver_name"`
	ReceiverPhone  string    `json:"receiver_phone"`
	PackageDesc    string    `json:"package_desc"`
	PackageWeight  float64   `json:"weight_kg"`
	AcceptedAt     time.Time `json:"accepted_at"`
	OccurredAt     time.Time `json:"occurred_at"`
}

type BookingDispatchCancelledEvent struct {
	Event         string    `json:"event"`
	CorrelationID string    `json:"correlation_id,omitempty"`
	BookingID     string    `json:"booking_id"`
	Reason        string    `json:"reason"`
	CancelledAt   time.Time `json:"cancelled_at"`
	OccurredAt    time.Time `json:"occurred_at"`
}

// CustomerCancelTripInput carries data for CancelTripByCustomerTx (Phase 7L).
type CustomerCancelTripInput struct {
	TripID     string
	FromStatus TripStatus
	ReasonText string
	Now        time.Time
}

type ProviderLocationUpdatedEvent struct {
	Event      string    `json:"event"`
	ProviderID string    `json:"provider_id"`
	Lat        float64   `json:"lat"`
	Lng        float64   `json:"lng"`
	UpdatedAt  time.Time `json:"updated_at"`
	OccurredAt time.Time `json:"occurred_at"`
}

type TripAssignedEvent struct {
	Event         string     `json:"event"`
	CorrelationID string     `json:"correlation_id,omitempty"`
	TripID        string     `json:"trip_id"`
	BookingID     string     `json:"booking_id"`
	ProviderID    string     `json:"provider_id"`
	Status        TripStatus `json:"status"`
	OccurredAt    time.Time  `json:"occurred_at"`
}

type TripStatusUpdatedEvent struct {
	Event      string     `json:"event"`
	TripID     string     `json:"trip_id"`
	BookingID  string     `json:"booking_id"`
	ProviderID string     `json:"provider_id"`
	FromStatus TripStatus `json:"from_status"`
	ToStatus   TripStatus `json:"to_status"`
	OccurredAt time.Time  `json:"occurred_at"`
}

// ── Phase 7J–7K outbound event topics ────────────────────────────────────────

const (
	TopicTripCompleted          = "trip.completed"
	TopicTripCancelled          = "trip.cancelled"
	TopicVerificationSuspension = "verification.suspension_flag"
)

// TripCompletedEvent is published after a trip is successfully completed (Phase 7J).
// Consumers: earnings-service, payment-wallet-service, availability-service, customer-service.
type TripCompletedEvent struct {
	Event         string    `json:"event"`
	CorrelationID string    `json:"correlation_id,omitempty"`
	TripID        string    `json:"trip_id"`
	BookingID     string    `json:"booking_id"`
	ProviderID    string    `json:"provider_id"`
	CustomerID    string    `json:"customer_id"`
	FareAmount    int64     `json:"fare_amount"`
	Currency      string    `json:"currency"`
	CompletedAt   time.Time `json:"completed_at"`
	OccurredAt    time.Time `json:"occurred_at"`
}

// TripCancelledEvent is published after a trip is cancelled (Phase 7K).
// Consumers: availability-service (set provider back online), payment-wallet-service (freeze/release escrow).
type TripCancelledEvent struct {
	Event                      string      `json:"event"`
	CorrelationID              string      `json:"correlation_id,omitempty"`
	TripID                     string      `json:"trip_id"`
	BookingID                  string      `json:"booking_id"`
	ProviderID                 string      `json:"provider_id"`
	CustomerID                 string      `json:"customer_id"`
	CancelledBy                CancelledBy `json:"cancelled_by"`
	ReasonCode                 string      `json:"reason_code"`
	ReasonText                 string      `json:"reason_text,omitempty"`
	PenaltyApplied             bool        `json:"penalty_applied"`
	RequiresAdminInvestigation bool        `json:"requires_admin_investigation"`
	CancelledAt                time.Time   `json:"cancelled_at"`
	OccurredAt                 time.Time   `json:"occurred_at"`
}

// SuspensionFlagEvent is published when a provider reaches 3+ cancellations in 30 days (Phase 7K).
type SuspensionFlagEvent struct {
	Event       string    `json:"event"`
	ProviderID  string    `json:"provider_id"`
	Reason      string    `json:"reason"`
	Count30Days int       `json:"count_30_days"`
	OccurredAt  time.Time `json:"occurred_at"`
}

// ── Phase 7J–7K input/response types ─────────────────────────────────────────

// CompleteTripInput carries the data needed for the CompleteTripTx DB transaction.
type CompleteTripInput struct {
	TripID     string
	ProviderID string
	Now        time.Time
}

// CancelTripInput carries the data needed for the CancelTripTx DB transaction.
type CancelTripInput struct {
	TripID         string
	ProviderID     string
	FromStatus     TripStatus
	ReasonCode     string
	ReasonText     string
	PenaltyApplied bool
	Now            time.Time
}

// CompleteResponse is returned by POST /api/v1/provider/trips/:id/complete.
type CompleteResponse struct {
	ID          string     `json:"id"`
	BookingID   string     `json:"booking_id"`
	Status      TripStatus `json:"status"`
	FareAmount  int64      `json:"fare_amount"`
	Currency    string     `json:"currency"`
	CompletedAt time.Time  `json:"completed_at"`
	Message     string     `json:"message"`
}

// CancelRequest is the JSON body for POST /api/v1/provider/trips/:id/cancel.
type CancelRequest struct {
	ReasonCode string `json:"reason_code"`
	ReasonText string `json:"reason_text"`
}

// CancelResponse is returned by POST /api/v1/provider/trips/:id/cancel.
type CancelResponse struct {
	ID                         string     `json:"id"`
	BookingID                  string     `json:"booking_id"`
	Status                     TripStatus `json:"status"`
	ReasonCode                 string     `json:"reason_code"`
	CancelledAt                time.Time  `json:"cancelled_at"`
	PenaltyApplied             bool       `json:"penalty_applied"`
	RequiresAdminInvestigation bool       `json:"requires_admin_investigation"`
	Warning                    string     `json:"warning,omitempty"`
}

// ── Phase 7F–7N outbound event topics ────────────────────────────────────────

const (
	TopicTripCreated         = "trip.created"
	TopicTripProviderArrived = "trip.provider_arrived"
	TopicTripStarted         = "trip.started"
	TopicTripProofSubmitted  = "trip.proof_submitted"
)

// TripCreatedEvent is published when a trip is first created from request.accepted (Phase 7M).
// Consumers: customer-service (marks booking as accepted), notification-service.
// NOTE: duplicate request.accepted events may publish duplicate trip.created events;
// consumers should deduplicate on trip_id.
type TripCreatedEvent struct {
	Event         string    `json:"event"`
	CorrelationID string    `json:"correlation_id,omitempty"`
	TripID        string    `json:"trip_id"`
	BookingID     string    `json:"booking_id"`
	ProviderID    string    `json:"provider_id"`
	CustomerID    string    `json:"customer_id"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	OccurredAt    time.Time `json:"occurred_at"`
}

// TripProviderArrivedEvent is published when the provider marks arrived at pickup (Phase 7F).
type TripProviderArrivedEvent struct {
	Event         string    `json:"event"`
	CorrelationID string    `json:"correlation_id,omitempty"`
	TripID        string    `json:"trip_id"`
	BookingID     string    `json:"booking_id"`
	ProviderID    string    `json:"provider_id"`
	CustomerID    string    `json:"customer_id"`
	PickupAddress string    `json:"pickup_address"`
	PickupLat     float64   `json:"pickup_lat"`
	PickupLng     float64   `json:"pickup_lng"`
	ArrivedAt     time.Time `json:"arrived_at"`
	OccurredAt    time.Time `json:"occurred_at"`
}

// TripStartedEvent is published when the provider starts the delivery (Phase 7G).
// Availability subscriber uses this to set provider status = busy.
type TripStartedEvent struct {
	Event          string    `json:"event"`
	CorrelationID  string    `json:"correlation_id,omitempty"`
	TripID         string    `json:"trip_id"`
	BookingID      string    `json:"booking_id"`
	ProviderID     string    `json:"provider_id"`
	CustomerID     string    `json:"customer_id"`
	PickupLat      float64   `json:"pickup_lat"`
	PickupLng      float64   `json:"pickup_lng"`
	DropoffAddress string    `json:"dropoff_address"`
	DropoffLat     float64   `json:"dropoff_lat"`
	DropoffLng     float64   `json:"dropoff_lng"`
	StartedAt      time.Time `json:"started_at"`
	OccurredAt     time.Time `json:"occurred_at"`
}

// TripProofSubmittedEvent is published after successful proof submission (Phase 7H).
// Proof refs use private object references and never raw filesystem paths.
type TripProofSubmittedEvent struct {
	Event         string    `json:"event"`
	CorrelationID string    `json:"correlation_id,omitempty"`
	TripID        string    `json:"trip_id"`
	BookingID     string    `json:"booking_id"`
	ProviderID    string    `json:"provider_id"`
	CustomerID    string    `json:"customer_id"`
	PhotoRef      string    `json:"photo_ref"`
	SignatureRef  string    `json:"signature_ref"`
	ReceiverName  string    `json:"receiver_name"`
	ReceiverPhone string    `json:"receiver_phone"`
	SubmittedAt   time.Time `json:"submitted_at"`
	OccurredAt    time.Time `json:"occurred_at"`
}

// ── Phase 7F–7H response types ───────────────────────────────────────────────

type ArrivedResponse struct {
	ID        string     `json:"id"`
	BookingID string     `json:"booking_id"`
	Status    TripStatus `json:"status"`
	ArrivedAt time.Time  `json:"arrived_at"`
	Message   string     `json:"message"`
}

type StartTripResponse struct {
	ID             string     `json:"id"`
	BookingID      string     `json:"booking_id"`
	Status         TripStatus `json:"status"`
	StartedAt      time.Time  `json:"started_at"`
	DropoffAddress string     `json:"dropoff_address"`
	DropoffLat     float64    `json:"dropoff_lat"`
	DropoffLng     float64    `json:"dropoff_lng"`
}

type ProofSubmitResponse struct {
	TripID        string    `json:"trip_id"`
	PhotoRef      string    `json:"photo_ref"`
	SignatureRef  string    `json:"signature_ref"`
	ReceiverName  string    `json:"receiver_name"`
	ReceiverPhone string    `json:"receiver_phone"`
	SubmittedAt   time.Time `json:"submitted_at"`
	Message       string    `json:"message"`
}

// SubmitProofDBInput carries proof and trip update data for the SubmitProofTx transaction (Phase 7H).
type SubmitProofDBInput struct {
	TripID        string
	ProviderID    string
	PhotoRef      string
	SignatureRef  string
	ReceiverName  string
	ReceiverPhone string
	Now           time.Time
}

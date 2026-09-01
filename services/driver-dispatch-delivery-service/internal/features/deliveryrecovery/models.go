package deliveryrecovery

import "time"

const PolicyVersion = "dispatch_delivery_recovery_v1"

type AttemptState string

const (
	AttemptGracePeriod         AttemptState = "GRACE_PERIOD"
	AttemptReceiverUnavailable AttemptState = "RECEIVER_UNAVAILABLE"
	AttemptRecoverySelected    AttemptState = "RECOVERY_SELECTED"
	AttemptResolved            AttemptState = "RESOLVED"
	AttemptCancelled           AttemptState = "CANCELLED"
)

type RecoveryAction string

const (
	RetrySameDay        RecoveryAction = "RETRY_SAME_DAY"
	Reschedule          RecoveryAction = "RESCHEDULE"
	AlternativeReceiver RecoveryAction = "ALTERNATIVE_RECEIVER"
	ReturnToSender      RecoveryAction = "RETURN_TO_SENDER"
	HubHold             RecoveryAction = "HUB_HOLD"
	OperationsReview    RecoveryAction = "OPERATIONS_REVIEW"
)

type Attempt struct {
	ID             string           `json:"attempt_id"`
	TripID         string           `json:"trip_id"`
	BookingID      string           `json:"booking_id,omitempty"`
	ProviderID     string           `json:"provider_id"`
	CustomerID     string           `json:"customer_id,omitempty"`
	AttemptNumber  int              `json:"attempt_number"`
	State          AttemptState     `json:"state"`
	ArrivedAt      time.Time        `json:"arrived_at"`
	GraceExpiresAt time.Time        `json:"grace_expires_at"`
	FailureReason  string           `json:"failure_reason,omitempty"`
	AllowedActions []RecoveryAction `json:"allowed_actions"`
	PolicyVersion  string           `json:"policy_version"`
	SourceVersion  int64            `json:"source_version"`
	Replayed       bool             `json:"replayed"`
}

type StartAttemptCommand struct {
	TripID         string
	ProviderID     string
	IdempotencyKey string
}

type ContactCommand struct {
	AttemptID      string
	ProviderID     string
	Method         string
	Outcome        string
	IdempotencyKey string
}

type FailAttemptCommand struct {
	AttemptID      string
	ProviderID     string
	ReasonCode     string
	IdempotencyKey string
}

type ContactAttempt struct {
	ID          string    `json:"contact_attempt_id"`
	AttemptID   string    `json:"attempt_id"`
	Method      string    `json:"method"`
	Outcome     string    `json:"outcome"`
	AttemptedAt time.Time `json:"attempted_at"`
	Replayed    bool      `json:"replayed"`
}

type SelectRecoveryCommand struct {
	BookingID                  string
	CustomerID                 string
	Action                     RecoveryAction `json:"action"`
	ScheduledFor               *time.Time     `json:"scheduled_for,omitempty"`
	AlternativeReceiverContact string         `json:"alternative_receiver_contact,omitempty"`
	HubRef                     string         `json:"hub_ref,omitempty"`
	HubAuthorityRef            string         `json:"-"`
	FinancialAuthorizationRef  string         `json:"financial_authorization_ref,omitempty"`
	AdditionalFeeKobo          int64          `json:"additional_fee_kobo,omitempty"`
	ExpectedTripVersion        int64          `json:"expected_trip_version"`
	IdempotencyKey             string         `json:"-"`
}

type RecoveryResult struct {
	ID                     string         `json:"recovery_action_id"`
	AttemptID              string         `json:"attempt_id"`
	TripID                 string         `json:"trip_id"`
	BookingID              string         `json:"booking_id"`
	Action                 RecoveryAction `json:"action"`
	State                  string         `json:"state"`
	ScheduledFor           *time.Time     `json:"scheduled_for,omitempty"`
	AlternativeReceiverRef string         `json:"alternative_receiver_ref,omitempty"`
	HubRef                 string         `json:"hub_ref,omitempty"`
	ReturnMissionID        string         `json:"return_mission_id,omitempty"`
	ReturnBookingID        string         `json:"return_booking_id,omitempty"`
	FinancialState         string         `json:"financial_state"`
	FinancialOperationRef  string         `json:"financial_operation_ref,omitempty"`
	PolicyVersion          string         `json:"policy_version"`
	Replayed               bool           `json:"replayed"`
}

type PendingReturnAssignment struct {
	WorkflowID   string
	MissionID    string
	OperationRef string
	AttemptCount int
}

type IncidentCommand struct {
	TripID            string
	ProviderID        string
	ReasonCode        string `json:"reason_code"`
	SafeNote          string `json:"safe_note,omitempty"`
	PreferredStrategy string `json:"preferred_strategy,omitempty"`
	IdempotencyKey    string `json:"-"`
}

type Incident struct {
	ID                  string    `json:"incident_id"`
	TripID              string    `json:"trip_id"`
	ReasonCode          string    `json:"reason_code"`
	State               string    `json:"state"`
	ReplacementStrategy string    `json:"replacement_strategy"`
	SourceVersion       int64     `json:"source_version"`
	CreatedAt           time.Time `json:"created_at"`
	Replayed            bool      `json:"replayed"`
	MissionID           string    `json:"-"`
}

type HubCheckpointCommand struct {
	TripID         string
	ProviderID     string
	Stage          string   `json:"stage"`
	ParcelCodes    []string `json:"parcel_codes"`
	MediaRef       string   `json:"media_ref"`
	IdempotencyKey string   `json:"-"`
}

type HubCheckpoint struct {
	OperationID string    `json:"operation_id"`
	TripID      string    `json:"trip_id"`
	HubRef      string    `json:"hub_ref"`
	Stage       string    `json:"stage"`
	State       string    `json:"state"`
	EvidenceRef string    `json:"evidence_ref"`
	RecordedAt  time.Time `json:"recorded_at"`
	Replayed    bool      `json:"replayed"`
}

type HubCheckpointContext struct {
	OperationID string
	HubRef      string
	State       string
	MissionRef  string
	ManifestRef string
	ParcelRefs  []string
	ParcelCodes []string
}

type EmergencyRecoveryCommand struct {
	ExpectedTripVersion int64 `json:"expected_trip_version"`
	TripID              string
	ActorRef            string `json:"operator_ref"`
	ReasonCode          string `json:"reason_code"`
	SafeNote            string `json:"safe_note,omitempty"`
	IdempotencyKey      string `json:"-"`
}

type EmergencyRecovery struct {
	ID         string    `json:"emergency_recovery_id"`
	TripID     string    `json:"trip_id"`
	IncidentID string    `json:"incident_id"`
	ActorRef   string    `json:"actor_ref"`
	ReasonCode string    `json:"reason_code"`
	State      string    `json:"state"`
	CreatedAt  time.Time `json:"created_at"`
	Replayed   bool      `json:"replayed"`
}

type TripContext struct {
	TripID         string
	BookingID      string
	ProviderID     string
	CustomerID     string
	Status         string
	SourceVersion  int64
	PickupAddress  string
	DropoffAddress string
	PickupLat      float64
	PickupLng      float64
	DropoffLat     float64
	DropoffLng     float64
	Currency       string
	ReceiverName   string
	ReceiverPhone  string
}

type RouteDecision struct {
	Reference       string
	Provider        string
	EvidenceVersion string
	DistanceMeters  int64
	DurationSeconds int64
	Snapshot        map[string]any
}

type Policy struct {
	Enabled              bool
	ReturnEnabled        bool
	HubHoldEnabled       bool
	ReplacementEnabled   bool
	GracePeriod          time.Duration
	MaximumSameDayWindow time.Duration
}

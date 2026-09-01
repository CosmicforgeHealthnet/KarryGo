package assignment

import "time"

type Strategy string

const (
	BroadcastFirstAccept Strategy = "BROADCAST_FIRST_ACCEPT"
	SequentialNearest    Strategy = "SEQUENTIAL_NEAREST"
	OrganizationDispatch Strategy = "ORGANIZATION_DISPATCH"
	AdminAssisted        Strategy = "ADMIN_ASSISTED"
)

func (s Strategy) Valid() bool {
	switch s {
	case BroadcastFirstAccept, SequentialNearest, OrganizationDispatch, AdminAssisted:
		return true
	default:
		return false
	}
}

type QueueStatus string

const (
	QueueQueued          QueueStatus = "QUEUED"
	QueueRetrying        QueueStatus = "RETRYING"
	QueueWaitingCustomer QueueStatus = "WAITING_CUSTOMER"
	QueueScheduled       QueueStatus = "SCHEDULED"
	QueueAssigned        QueueStatus = "ASSIGNED"
	QueueExpired         QueueStatus = "EXPIRED"
	QueueCancelled       QueueStatus = "CANCELLED"
	QueueFailed          QueueStatus = "FAILED"
)

type Candidate struct {
	ProviderID string         `json:"provider_id"`
	Rank       int            `json:"rank"`
	DistanceM  int64          `json:"distance_meters"`
	Snapshot   map[string]any `json:"snapshot,omitempty"`
}

type StartRequest struct {
	MissionID                string     `json:"mission_id"`
	Strategy                 Strategy   `json:"strategy,omitempty"`
	OrganizationID           string     `json:"organization_id,omitempty"`
	Priority                 int        `json:"priority,omitempty"`
	PriorityReason           string     `json:"-"`
	PriorityPolicyVersion    string     `json:"-"`
	QuoteRef                 string     `json:"quote_ref,omitempty"`
	QuoteExpiresAt           *time.Time `json:"quote_expires_at,omitempty"`
	QueueExpiresAt           time.Time  `json:"queue_expires_at"`
	ScheduledFor             *time.Time `json:"scheduled_for,omitempty"`
	ProviderID               string     `json:"provider_id,omitempty"`
	ActorRef                 string     `json:"actor_ref,omitempty"`
	ReasonCode               string     `json:"reason_code,omitempty"`
	Note                     string     `json:"note,omitempty"`
	CorrelationID            string     `json:"correlation_id,omitempty"`
	ExpectedMissionVersion   int64      `json:"expected_version,omitempty"`
	ServiceTier              string     `json:"service_tier,omitempty"`
	ServiceabilityRef        string     `json:"serviceability_ref,omitempty"`
	CustomerDecisionDeadline *time.Time `json:"customer_decision_deadline,omitempty"`
	InitialRadiusKM          float64    `json:"-"`
	MaxRadiusKM              float64    `json:"-"`
	IdempotencyKey           string     `json:"-"`
}

type Offer struct {
	ID          string    `json:"offer_id"`
	MissionID   string    `json:"mission_id"`
	ProviderID  string    `json:"provider_id"`
	Status      string    `json:"status"`
	Purpose     string    `json:"purpose,omitempty"`
	IncidentRef string    `json:"incident_ref,omitempty"`
	Rank        int       `json:"rank"`
	OfferedAt   time.Time `json:"offered_at"`
	ExpiresAt   time.Time `json:"expires_at"`
	Version     int64     `json:"version"`
}

type Assignment struct {
	MissionID string   `json:"mission_id"`
	Strategy  Strategy `json:"strategy"`
	Status    string   `json:"status"`
	Offer     *Offer   `json:"offer,omitempty"`
	TripID    string   `json:"trip_id,omitempty"`
	Replayed  bool     `json:"replayed"`
}

type Decision struct {
	IdempotencyKey  string `json:"-"`
	ExpectedVersion int64  `json:"expected_version"`
	Reason          string `json:"reason,omitempty"`
}

type QueueOption struct {
	Option         string     `json:"option"`
	ScheduledFor   *time.Time `json:"scheduled_for,omitempty"`
	QuoteRef       string     `json:"quote_ref,omitempty"`
	QuoteExpiresAt *time.Time `json:"quote_expires_at,omitempty"`
	IdempotencyKey string     `json:"-"`
	SourceRef      string     `json:"source_ref,omitempty"`
	SourceVersion  int64      `json:"source_version,omitempty"`
}

type QueueEntry struct {
	ID            string      `json:"queue_id"`
	MissionID     string      `json:"mission_id"`
	Status        QueueStatus `json:"status"`
	Priority      int         `json:"priority"`
	RetryCount    int         `json:"retry_count"`
	NextAttemptAt time.Time   `json:"next_attempt_at"`
	ExpiresAt     time.Time   `json:"expires_at"`
	Version       int64       `json:"version"`
	Replayed      bool        `json:"replayed"`
}
type PendingWinner struct {
	MissionID  string
	ProviderID string
}
type PendingQueueFinancial struct {
	QueueID      string
	MissionID    string
	BookingID    string
	OperationRef string
	AttemptCount int
}
type OfferAuthorityContext struct {
	MissionID      string
	Strategy       Strategy
	OrganizationID string
}

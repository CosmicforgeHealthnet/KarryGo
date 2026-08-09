package integrations

import "time"

const (
	APIClientStatusActive  = "ACTIVE"
	APIClientStatusRevoked = "REVOKED"
	WebhookStatusActive    = "ACTIVE"
	WebhookStatusDisabled  = "DISABLED"
	DeliveryPending        = "PENDING"
	DeliveryDelivering     = "DELIVERING"
	DeliveryRetryScheduled = "RETRY_SCHEDULED"
	DeliverySucceeded      = "SUCCEEDED"
	DeliveryFailed         = "FAILED"
	DeliveryDeadLettered   = "DEAD_LETTERED"

	JobTypeWebhookDelivery = "webhook.delivery"
)

type APIClient struct {
	ID, OrganizationID, Name, ClientID, Status, CreatedBy string
	Scopes                                                []string
	IPAllowlist                                           []string
	ExpiresAt, LastUsedAt, RotatedAt, RevokedAt           *time.Time
	SecretVersion                                         int
	Version                                               int64
	CreatedAt, UpdatedAt                                  time.Time
}

type WebhookSubscription struct {
	ID, OrganizationID, EndpointURL, Status, CreatedBy string
	EventTypes                                         []string
	FailureCount, SecretVersion                        int
	DisabledAt                                         *time.Time
	Version                                            int64
	CreatedAt, UpdatedAt                               time.Time
}

type WebhookDelivery struct {
	ID, OrganizationID, SubscriptionID, SourceEventID, EventType string
	Status, ErrorClass, ErrorCode, CorrelationID, JobID          string
	ReplayOfDeliveryID, ResponseExcerpt                          string
	AttemptNo, ResponseStatus                                    int
	ScheduledAt, CreatedAt, UpdatedAt                            time.Time
	DeliveredAt, NextAttemptAt                                   *time.Time
	Version                                                      int64
}

type AuthenticatedClient struct {
	APIClientID, ClientID, OrganizationID string
	Scopes                                []string
	SecretVersion                         int
}

type PublicWebhookEnvelope struct {
	EventID        string    `json:"eventId"`
	EventType      string    `json:"eventType"`
	SchemaVersion  int       `json:"schemaVersion"`
	OrganizationID string    `json:"organizationId"`
	CorrelationID  string    `json:"correlationId"`
	OccurredAt     time.Time `json:"occurredAt"`
	Data           any       `json:"data"`
}

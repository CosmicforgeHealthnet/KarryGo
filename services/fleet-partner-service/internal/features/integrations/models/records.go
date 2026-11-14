package models

import "time"

// ClientCredential is what authenticating one API client needs.
type ClientCredential struct {
	APIClientID, ClientID, OrganizationID, Status string
	ExpiresAt                                     *time.Time
	Scopes, IPAllowlist                           []string
	SecretHash, PreviousHash                      []byte
	SecretVersion                                 int
	PreviousVersion                               *int
	PreviousValidUntil                            *time.Time
}

// OutboxEvent is one committed outbox event awaiting webhook fanout.
type OutboxEvent struct {
	ID  string
	Raw []byte
}

// DeliveryContext is everything one webhook delivery attempt needs.
type DeliveryContext struct {
	SubscriptionID, EventID, EventType, Endpoint, CorrelationID, Status string
	CurrentCipher, PreviousCipher, RawEvent                             []byte
	CurrentVersion                                                      int
	PreviousVersion                                                     *int
	PreviousUntil                                                       *time.Time
}

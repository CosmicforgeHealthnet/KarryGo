// Package models holds the Section 55 integration domain: organization API
// clients, webhook subscriptions and deliveries, and the pure credential,
// signing and payload rules around them.
package models

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"sort"
	"strings"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

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

// ScopeIntegrationContextRead is the only scope an organization API client
// may hold.
const ScopeIntegrationContextRead = "integration.context.read"

var allowedClientScopes = map[string]bool{ScopeIntegrationContextRead: true}

type Actor struct {
	Tenant          tenant.Context
	IdentityID      string
	SessionID       string
	StepUpReference string
}

type CreateAPIClientInput struct {
	Name, IdempotencyKey string
	Scopes               []string
	IPAllowlist          []string
	ExpiresAt            *time.Time
}

type RotateAPIClientInput struct {
	IdempotencyKey, StepUpReference string
	ExpectedVersion                 int64
	Overlap                         time.Duration
}

type CreateWebhookInput struct {
	EndpointURL, IdempotencyKey string
	EventTypes                  []string
}

type UpdateWebhookInput struct {
	EndpointURL, Status, IdempotencyKey string
	EventTypes                          []string
	ExpectedVersion                     int64
}

// ValidateAPIClientInput normalizes an API client request: a safe name, an
// exact scope allowlist, canonical sorted CIDRs and a future expiry.
func ValidateAPIClientInput(in CreateAPIClientInput) (string, []string, []string, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || len(name) > 120 || strings.ContainsAny(name, "\r\n\x00") {
		return "", nil, nil, apperrors.BadRequest("API client name is invalid.", nil)
	}
	scopes, err := ExactAllowlist(in.Scopes, allowedClientScopes, "scope")
	if err != nil {
		return "", nil, nil, err
	}
	// Never nil: a nil slice is sent as NULL into the NOT NULL ip_allowlist column.
	cidrs := []string{}
	seen := map[string]bool{}
	for _, raw := range in.IPAllowlist {
		_, network, parseErr := net.ParseCIDR(strings.TrimSpace(raw))
		if parseErr != nil {
			return "", nil, nil, apperrors.BadRequest("API client IP allowlist contains an invalid CIDR.", nil)
		}
		canonical := network.String()
		if !seen[canonical] {
			seen[canonical] = true
			cidrs = append(cidrs, canonical)
		}
	}
	sort.Strings(cidrs)
	if in.ExpiresAt != nil && !in.ExpiresAt.After(time.Now().UTC()) {
		return "", nil, nil, apperrors.BadRequest("API client expiry must be in the future.", nil)
	}
	return name, scopes, cidrs, nil
}

// ExactAllowlist trims, de-duplicates and sorts values, refusing any not in
// allowed and an empty result.
func ExactAllowlist(values []string, allowed map[string]bool, label string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" || !allowed[value] {
			return nil, apperrors.BadRequest("Unsupported "+label+".", nil)
		}
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return nil, apperrors.BadRequest("At least one "+label+" is required.", nil)
	}
	sort.Strings(out)
	return out, nil
}

// Fingerprint is the SHA-256 of value's JSON encoding.
func Fingerprint(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// ContainsExact reports whether expected is one of values.
func ContainsExact(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

// IPAllowed reports whether raw is inside one of cidrs.
func IPAllowed(raw string, cidrs []string) bool {
	ip := net.ParseIP(strings.TrimSpace(raw))
	if ip == nil {
		return false
	}
	for _, rawCIDR := range cidrs {
		_, network, err := net.ParseCIDR(rawCIDR)
		if err == nil && network.Contains(ip) {
			return true
		}
	}
	return false
}

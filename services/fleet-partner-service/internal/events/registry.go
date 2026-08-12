package events

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type Direction string

type Visibility string

const (
	Produced                    Direction  = "PRODUCED"
	Consumed                    Direction  = "CONSUMED"
	VisibilityPublicIntegration Visibility = "PUBLIC_INTEGRATION"
	VisibilityInternalService   Visibility = "INTERNAL_SERVICE"
	VisibilityPlatformOnly      Visibility = "PLATFORM_ONLY"
)

// ContractPayload is the minimized common wire payload. Individual domains
// may embed/extend it, but a registry entry always has a concrete factory and
// never decodes into map[string]any.
type ContractPayload struct {
	SubjectReference      string `json:"subjectReference"`
	SourceVersion         int64  `json:"sourceVersion"`
	Status                string `json:"status,omitempty"`
	ReasonCode            string `json:"reasonCode,omitempty"`
	BranchID              string `json:"branchId,omitempty"`
	RequestID             string `json:"requestId,omitempty"`
	TripID                string `json:"tripId,omitempty"`
	ProviderID            string `json:"providerId,omitempty"`
	VehicleID             string `json:"vehicleId,omitempty"`
	WalletEntryID         string `json:"walletEntryId,omitempty"`
	SettlementID          string `json:"settlementId,omitempty"`
	CaseID                string `json:"caseId,omitempty"`
	MediaObjectID         string `json:"mediaObjectId,omitempty"`
	AvailabilityState     string `json:"availabilityState,omitempty"`
	VerificationState     string `json:"verificationState,omitempty"`
	Available             *bool  `json:"available,omitempty"`
	Priority              string `json:"priority,omitempty"`
	OwnerReference        string `json:"ownerReference,omitempty"`
	UploadStatus          string `json:"uploadStatus,omitempty"`
	SafetyStatus          string `json:"safetyStatus,omitempty"`
	DeletionState         string `json:"deletionState,omitempty"`
	AmountMinor           *int64 `json:"amountMinor,omitempty"`
	Currency              string `json:"currency,omitempty"`
	OrganizationID        string `json:"organizationId,omitempty"`
	OwnerService          string `json:"ownerService,omitempty"`
	OwnerType             string `json:"ownerType,omitempty"`
	UploadPurpose         string `json:"uploadPurpose,omitempty"`
	SignatureValidation   string `json:"signatureValidation,omitempty"`
	DetectedMIME          string `json:"detectedMime,omitempty"`
	SizeBytes             int64  `json:"sizeBytes,omitempty"`
	ChecksumSHA256        string `json:"checksumSha256,omitempty"`
	CapturedAt            string `json:"capturedAt,omitempty"`
	SourceTimestamp       string `json:"sourceTimestamp,omitempty"`
	CaptureMethod         string `json:"captureMethod,omitempty"`
	DeviceReference       string `json:"deviceReference,omitempty"`
	ActorReference        string `json:"actorReference,omitempty"`
	OriginalMIMEType      string `json:"originalMimeType,omitempty"`
	OriginalSizeBytes     int64  `json:"originalSizeBytes,omitempty"`
	IntegrityHash         string `json:"integrityHash,omitempty"`
	ValidationFailureCode string `json:"validationFailureCode,omitempty"`
	SafetyReason          string `json:"safetyReason,omitempty"`
	FailureCode           string `json:"failureCode,omitempty"`
}

type EventDefinition struct {
	Type            string
	Direction       Direction
	SchemaVersion   int
	AggregateType   string
	Description     string
	PlatformScoped  bool
	Producers       []string
	MaxPayloadBytes int
	TenantRequired  bool
	PayloadFactory  func() any
	ForbiddenFields []string
	Visibility      Visibility
}

// IntegrationContractPayload is the minimal contract for API-client and
// webhook lifecycle events. It intentionally contains only opaque references,
// status and allowlisted scope/event names; credential material has no field.
type IntegrationContractPayload struct {
	SubjectReference string   `json:"subjectReference"`
	SourceVersion    int64    `json:"sourceVersion"`
	Status           string   `json:"status"`
	Scopes           []string `json:"scopes,omitempty"`
	EventTypes       []string `json:"eventTypes,omitempty"`
	EventID          string   `json:"eventId,omitempty"`
	Attempt          int      `json:"attempt,omitempty"`
}

var registry = map[string]EventDefinition{}

func Register(def EventDefinition) {
	if def.Type == "" {
		panic("events: cannot register an event definition with an empty type")
	}
	if def.SchemaVersion < 1 {
		panic(fmt.Sprintf("events: %s: schema version must be positive", def.Type))
	}
	if def.AggregateType == "" {
		panic(fmt.Sprintf("events: %s: aggregate type is required", def.Type))
	}
	if _, exists := registry[def.Type]; exists {
		panic(fmt.Sprintf("events: event type %q is already registered", def.Type))
	}
	if def.Visibility == "" {
		def.Visibility = VisibilityInternalService
	}
	registry[def.Type] = def
}

func Lookup(eventType string) (EventDefinition, bool) { def, ok := registry[eventType]; return def, ok }

func Registered() []EventDefinition {
	out := make([]EventDefinition, 0, len(registry))
	for _, def := range registry {
		out = append(out, def)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Type < out[j].Type })
	return out
}

// PublicIntegrationTypes returns the exact, allowlisted event names an
// organization webhook may subscribe to. Arbitrary strings and internal-only
// lifecycle events never cross this boundary.
func PublicIntegrationTypes() []string {
	var out []string
	for _, def := range registry {
		if def.Direction == Produced && def.Visibility == VisibilityPublicIntegration {
			out = append(out, def.Type)
		}
	}
	sort.Strings(out)
	return out
}

func IsPublicIntegration(eventType string) bool {
	def, ok := Lookup(eventType)
	return ok && def.Direction == Produced && def.Visibility == VisibilityPublicIntegration
}

// ValidateEnvelope is the single routing and authorization boundary used by
// HTTP and Redis ingestion. It performs exact producer matching; substrings
// and unknown generic events are never accepted.
func ValidateEnvelope(envelope Envelope) error {
	def, ok := Lookup(envelope.EventType)
	if !ok {
		return fmt.Errorf("events: unregistered event type %q", envelope.EventType)
	}
	if envelope.SchemaVersion != def.SchemaVersion || envelope.AggregateType != def.AggregateType {
		return fmt.Errorf("events: contract mismatch for %s", envelope.EventType)
	}
	if def.TenantRequired && envelope.OrganizationID == "" {
		return fmt.Errorf("events: organization is required")
	}
	allowed := false
	for _, producer := range def.Producers {
		if envelope.Producer == producer {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("events: producer %q is not authorized for %s", envelope.Producer, envelope.EventType)
	}
	if def.MaxPayloadBytes > 0 && len(envelope.Payload) > def.MaxPayloadBytes {
		return fmt.Errorf("events: payload exceeds limit")
	}
	if def.PayloadFactory == nil {
		return fmt.Errorf("events: typed payload is not registered")
	}
	decoder := json.NewDecoder(bytes.NewReader(envelope.Payload))
	decoder.DisallowUnknownFields()
	payload := def.PayloadFactory()
	if err := decoder.Decode(payload); err != nil {
		return fmt.Errorf("events: invalid payload: %w", err)
	}
	common, ok := payload.(*ContractPayload)
	if ok && (strings.TrimSpace(common.SubjectReference) == "" || common.SourceVersion < 1) {
		return fmt.Errorf("events: subjectReference and sourceVersion are required")
	}
	integration, ok := payload.(*IntegrationContractPayload)
	if ok && (strings.TrimSpace(integration.SubjectReference) == "" || integration.SourceVersion < 1) {
		return fmt.Errorf("events: subjectReference and sourceVersion are required")
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(envelope.Payload, &raw); err != nil {
		return err
	}
	for _, field := range def.ForbiddenFields {
		if _, exists := raw[field]; exists {
			return fmt.Errorf("events: forbidden field %q", field)
		}
	}
	return nil
}

func init() {
	produced := map[string]string{
		"organization.created.v1": "organization", "organization.status_changed.v1": "organization",
		"organization.provider.provisioned.v1": "provider", "organization.provider.membership_changed.v1": "provider_membership",
		"organization.request.accepted.v1": "request", "organization.assignment.created.v1": "assignment",
		"organization.assignment.accepted.v1": "assignment", "organization.assignment.declined.v1": "assignment",
		"organization.assignment.expired.v1": "assignment", "organization.asset.blocked.v1": "asset",
		"corporate.order.confirmed.v1": "order", "corporate.shipment.status_changed.v1": "shipment",
		"corporate.invoice.issued.v1": "invoice",
	}
	consumed := map[string]struct {
		aggregate string
		producers []string
	}{
		"platform.request.offered.v1":      {"request", []string{"driver-dispatch-delivery-service", "driver-hauling-service", "taxi-service"}},
		"platform.request.cancelled.v1":    {"request", []string{"driver-dispatch-delivery-service", "driver-hauling-service", "taxi-service"}},
		"provider.availability.changed.v1": {"provider", []string{"driver-dispatch-delivery-service", "driver-hauling-service", "taxi-service"}},
		"provider.verification.changed.v1": {"provider", []string{"driver-dispatch-delivery-service", "driver-hauling-service", "verification-compliance-service"}},
		"trip.created.v1":                  {"trip", []string{"driver-dispatch-delivery-service", "driver-hauling-service", "taxi-service"}},
		"trip.status_changed.v1":           {"trip", []string{"driver-dispatch-delivery-service", "driver-hauling-service", "taxi-service"}},
		"trip.completed.v1":                {"trip", []string{"driver-dispatch-delivery-service", "driver-hauling-service", "taxi-service"}},
		"vehicle.verification.changed.v1":  {"vehicle", []string{"driver-dispatch-delivery-service", "driver-hauling-service", "verification-compliance-service"}},
		"wallet.entry.posted.v1":           {"wallet_entry", []string{"payment-wallet-service"}},
		"settlement.created.v1":            {"settlement", []string{"payment-wallet-service"}},
		"settlement.completed.v1":          {"settlement", []string{"payment-wallet-service"}},
		"support.case.changed.v1":          {"support_case", []string{"support-dispute-service"}},
		"media.asset.status_changed.v1":    {"MEDIA_OBJECT", []string{"media-file-service"}},
	}
	for eventType, aggregate := range produced {
		def := section26Definition(eventType, Produced, aggregate, []string{"fleet-partner-service"})
		if publicIntegrationEvents[eventType] {
			def.Visibility = VisibilityPublicIntegration
		}
		Register(def)
	}
	for eventType, item := range consumed {
		Register(section26Definition(eventType, Consumed, item.aggregate, item.producers))
	}
	for eventType, aggregate := range map[string]string{
		"api_client.created.v1": "organization_api_client", "api_client.rotated.v1": "organization_api_client",
		"api_client.revoked.v1": "organization_api_client", "webhook.subscription_created.v1": "webhook_subscription",
		"webhook.subscription_updated.v1": "webhook_subscription", "webhook.secret_rotated.v1": "webhook_subscription",
		"webhook.disabled.v1": "webhook_subscription", "webhook.delivery_succeeded.v1": "webhook_delivery",
		"webhook.delivery_failed.v1": "webhook_delivery", "webhook.delivery_replayed.v1": "webhook_delivery",
	} {
		Register(EventDefinition{Type: eventType, Direction: Produced, SchemaVersion: 1,
			AggregateType: aggregate, Description: "Section 55 integration lifecycle",
			Producers: []string{"fleet-partner-service"}, MaxPayloadBytes: 16 * 1024,
			TenantRequired: true, Visibility: VisibilityInternalService,
			PayloadFactory:  func() any { return &IntegrationContractPayload{} },
			ForbiddenFields: []string{"password", "token", "credential", "secret", "authorization", "signedUrl", "bankDetails", "evidence"}})
	}
}

var publicIntegrationEvents = map[string]bool{
	"organization.status_changed.v1":              true,
	"organization.provider.membership_changed.v1": true,
	"organization.assignment.created.v1":          true,
	"organization.assignment.accepted.v1":         true,
	"organization.assignment.declined.v1":         true,
	"organization.assignment.expired.v1":          true,
	"corporate.order.confirmed.v1":                true,
	"corporate.shipment.status_changed.v1":        true,
	"corporate.invoice.issued.v1":                 true,
}

func section26Definition(eventType string, direction Direction, aggregate string, producers []string) EventDefinition {
	return EventDefinition{Type: eventType, Direction: direction, SchemaVersion: 1, AggregateType: aggregate,
		Description: "Section 26 minimized contract", Producers: producers, MaxPayloadBytes: 32 * 1024,
		TenantRequired: true, PayloadFactory: func() any { return &ContractPayload{} },
		ForbiddenFields: []string{"password", "token", "credential", "signedUrl", "bankDetails", "privateMessage", "evidence"}}
}

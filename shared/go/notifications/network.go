package notifications

// Section 14: notifications for the network facts of Sections 13-17 --
// recovery, guarantee breaches, operator escalations and support actions --
// addressed to every audience those facts concern.
//
// Producers state WHAT happened and to WHOM; notification-service owns the
// copy (templates, migration 003) and the channels. A notification is always
// sent after the fact it announces has committed, and a failure to send it
// never undoes that fact.

import (
	"strings"
)

// Audiences the network intents add to the catalogue.
const (
	// RecipientFleetOrganization is a corporate fleet organization, addressed
	// by its organization id.
	RecipientFleetOrganization = "fleet_organization"
	// RecipientAdminOperations is the platform operations queue. Its one
	// recipient id is AdminOperationsQueue: an escalation is for whoever is on
	// duty, not for a named person.
	RecipientAdminOperations = "admin_operations"
	AdminOperationsQueue     = "network-operations"
)

// Network intents. Each is also its template key.
const (
	EventNetworkCustomerRecoveryStarted   = "network.customer.recovery_started"
	EventNetworkCustomerProviderReplaced  = "network.customer.provider_replaced"
	EventNetworkCustomerRecoveryFailed    = "network.customer.recovery_failed"
	EventNetworkCustomerGuaranteeBreached = "network.customer.guarantee_breached"
	EventNetworkCustomerSupportUpdate     = "network.customer.support_update"

	EventNetworkProviderJobReassigned       = "network.provider.job_reassigned"
	EventNetworkProviderReplacementAssigned = "network.provider.replacement_assigned"
	EventNetworkProviderSupportUpdate       = "network.provider.support_update"

	EventNetworkVendorDeliveryRecovery = "network.vendor.delivery_recovery"

	EventNetworkFleetProviderReplaced = "network.fleet.managed_provider_replaced"
	EventNetworkFleetRecoveryStarted  = "network.fleet.managed_recovery_started"

	EventNetworkAdminOperatorRequired = "network.admin.recovery_operator_required"
	EventNetworkAdminActionUnresolved = "network.admin.support_action_unresolved"
	// EventNetworkAdminSafetyDecline: a provider refused work for a safety
	// reason. Operations looks at the vehicle or route; it is not a mark
	// against the provider.
	EventNetworkAdminSafetyDecline = "network.admin.provider_safety_decline"
)

// NetworkEventTypes lists every intent, for template coverage checks.
func NetworkEventTypes() []string {
	return []string{
		EventNetworkCustomerRecoveryStarted, EventNetworkCustomerProviderReplaced,
		EventNetworkCustomerRecoveryFailed, EventNetworkCustomerGuaranteeBreached,
		EventNetworkCustomerSupportUpdate,
		EventNetworkProviderJobReassigned, EventNetworkProviderReplacementAssigned,
		EventNetworkProviderSupportUpdate,
		EventNetworkVendorDeliveryRecovery,
		EventNetworkFleetProviderReplaced, EventNetworkFleetRecoveryStarted,
		EventNetworkAdminOperatorRequired, EventNetworkAdminActionUnresolved,
		EventNetworkAdminSafetyDecline,
	}
}

// NetworkIntent is one audience's notification about one source fact.
type NetworkIntent struct {
	EventType     string
	RecipientType string
	RecipientID   string
	// SourceEventID names the fact. Together with the recipient and the
	// template it is the deduplication identity: the same fact announced to
	// the same person twice is one notification.
	SourceEventID       string
	SourceAggregateType string
	SourceAggregateID   string
	SourceVersion       int64
	CorrelationRef      string
	Data                map[string]interface{}
	Priority            string
}

// NetworkRequest builds the notification request for an intent.
//
// The idempotency key and the semantic dedupe index
// (source_service, source_event_id, event_type, recipient) agree, so a
// retried send and a re-published fact both land on the one message.
func NetworkRequest(sourceService string, intent NetworkIntent) Request {
	data := map[string]interface{}{}
	for key, value := range intent.Data {
		data[key] = value
	}
	return Request{
		IDempotencyKey: IdempotencyKey(sourceService, intent.EventType,
			intent.SourceEventID+":"+strings.TrimSpace(intent.RecipientType)+":"+strings.TrimSpace(intent.RecipientID)),
		SourceService:       sourceService,
		SourceAggregateType: intent.SourceAggregateType,
		SourceAggregateID:   intent.SourceAggregateID,
		SourceEventID:       intent.SourceEventID,
		SourceVersion:       intent.SourceVersion,
		CorrelationRef:      intent.CorrelationRef,
		EventType:           intent.EventType,
		Recipient:           Recipient{Type: intent.RecipientType, ID: intent.RecipientID},
		TemplateKey:         intent.EventType,
		Data:                data,
		TemplateData:        data,
		Priority:            intent.Priority,
	}
}

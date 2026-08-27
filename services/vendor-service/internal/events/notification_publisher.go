package events

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"cosmicforge/logistics/shared/go/notifications"
)

// NotificationPublisher adapts selected semantic Vendor outbox events to the
// platform Notification Service. It never contains transport credentials,
// destination addresses, or user-authored copy: only opaque recipients,
// controlled template keys and bounded semantic data cross the boundary.
type NotificationPublisher struct{ client notifications.Client }

func NewNotificationPublisher(client notifications.Client) *NotificationPublisher {
	return &NotificationPublisher{client: client}
}

type notificationIntent struct {
	eventType, template, recipientType, recipientField string
	channels                                           []string
	priority                                           string
	when                                               func(map[string]any) bool
}

var commerceNotificationCatalog = map[string][]notificationIntent{
	"commerce.vendor_order.created":               {{"commerce.vendor.new_order", "commerce.vendor.new_order", notifications.RecipientVendor, "business_ref", nil, notifications.PriorityHigh, nil}},
	"commerce.vendor_order.accepted":              {{"commerce.customer.order_accepted", "commerce.customer.order_accepted", notifications.RecipientCustomer, "customer_ref", nil, notifications.PriorityNormal, nil}},
	"commerce.vendor_order.rejected":              {{"commerce.customer.order_rejected", "commerce.customer.order_rejected", notifications.RecipientCustomer, "customer_ref", nil, notifications.PriorityHigh, nil}},
	"commerce.vendor_order.substitution_proposed": {{"commerce.customer.substitution", "commerce.customer.substitution", notifications.RecipientCustomer, "customer_ref", nil, notifications.PriorityHigh, nil}},
	"commerce.vendor_order.substitution_approved": {{"commerce.vendor.substitution_action", "commerce.vendor.substitution_action", notifications.RecipientVendor, "business_ref", nil, notifications.PriorityHigh, nil}},
	"commerce.vendor_order.substitution_rejected": {{"commerce.vendor.substitution_action", "commerce.vendor.substitution_action", notifications.RecipientVendor, "business_ref", nil, notifications.PriorityHigh, nil}},
	"commerce.vendor_order.ready":                 {{"commerce.customer.ready", "commerce.customer.ready", notifications.RecipientCustomer, "customer_ref", nil, notifications.PriorityHigh, nil}},
	"commerce.vendor_order.ready_for_pickup":      {{"commerce.customer.ready", "commerce.customer.ready", notifications.RecipientCustomer, "customer_ref", nil, notifications.PriorityHigh, nil}},
	"commerce.vendor_order.logistics_provider_assigned": {
		{"commerce.vendor.provider_assigned", "commerce.vendor.provider_assigned", notifications.RecipientVendor, "business_ref", nil, notifications.PriorityHigh, nil},
		{"commerce.customer.provider_assigned", "commerce.customer.provider_assigned", notifications.RecipientCustomer, "customer_ref", nil, notifications.PriorityNormal, nil},
	},
	"commerce.vendor_order.logistics_provider_reassigned": {
		{"commerce.vendor.provider_assigned", "commerce.vendor.provider_assigned", notifications.RecipientVendor, "business_ref", nil, notifications.PriorityHigh, nil},
		{"commerce.customer.provider_assigned", "commerce.customer.provider_assigned", notifications.RecipientCustomer, "customer_ref", nil, notifications.PriorityNormal, nil},
	},
	"commerce.vendor_order.logistics_provider_arrived": {{"commerce.vendor.provider_arrived", "commerce.vendor.provider_arrived", notifications.RecipientVendor, "business_ref", nil, notifications.PriorityHigh, nil}},
	"commerce.vendor_order.logistics_picked_up":        {{"commerce.vendor.pickup_confirmed", "commerce.vendor.pickup_confirmed", notifications.RecipientVendor, "business_ref", nil, notifications.PriorityHigh, nil}},
	"commerce.vendor_order.logistics_delivered":        {{"commerce.customer.delivered", "commerce.customer.delivered", notifications.RecipientCustomer, "customer_ref", nil, notifications.PriorityHigh, nil}},
	"commerce.vendor_order.refund_pending":             {{"commerce.vendor.refund_action", "commerce.vendor.refund_action", notifications.RecipientVendor, "business_ref", nil, notifications.PriorityHigh, nil}},
	"commerce.vendor_order.refunded": {
		{"commerce.vendor.refund_action", "commerce.vendor.refund_action", notifications.RecipientVendor, "business_ref", nil, notifications.PriorityHigh, nil},
		{"commerce.customer.refunded", "commerce.customer.refunded", notifications.RecipientCustomer, "customer_ref", nil, notifications.PriorityHigh, nil},
	},
	"commerce.vendor_order.partially_refunded": {
		{"commerce.vendor.refund_action", "commerce.vendor.refund_action", notifications.RecipientVendor, "business_ref", nil, notifications.PriorityHigh, nil},
		{"commerce.customer.refunded", "commerce.customer.refunded", notifications.RecipientCustomer, "customer_ref", nil, notifications.PriorityHigh, nil},
	},
	"commerce.scheduled_order.cutoff": {{"commerce.vendor.scheduled_cutoff", "commerce.vendor.scheduled_cutoff", notifications.RecipientVendor, "business_ref", nil, notifications.PriorityHigh, nil}},
	"commerce.vendor.support_action":  {{"commerce.vendor.support_action", "commerce.vendor.support_action", notifications.RecipientVendor, "business_ref", nil, notifications.PriorityHigh, nil}},
	"commerce.support_case.resolved": {
		{"commerce.customer.support_resolved", "commerce.customer.support_resolved", notifications.RecipientCustomer, "customer_ref", nil, notifications.PriorityHigh, nil},
		{"commerce.vendor.support_resolved", "commerce.vendor.support_resolved", notifications.RecipientVendor, "business_ref", nil, notifications.PriorityNormal, nil},
	},
	"commerce.fulfilment.plan_activated":         {{"commerce.customer.multi_delivery", "commerce.customer.multi_delivery", notifications.RecipientCustomer, "customer_ref", nil, notifications.PriorityNormal, func(p map[string]any) bool { return intAt(p, "missions") > 1 }}},
	"commerce.fulfilment.vendor_mission_updated": {{"commerce.vendor.rider_assignment", "commerce.vendor.rider_assignment", notifications.RecipientStaff, "staff_id", nil, notifications.PriorityHigh, func(p map[string]any) bool { return stringAt(p, "staff_id") != "" }}},
	"commerce.eta.updated":                       {{notifications.EventCommerceCustomerETAChanged, "commerce.customer_eta_changed", notifications.RecipientCustomer, "customer_ref", []string{notifications.ChannelWebSocket, notifications.ChannelInApp}, notifications.PriorityNormal, nil}},
	// Retain the accepted Section 20 event name while the new confidence-ETA
	// projection emits commerce.eta.updated. Both resolve to one controlled
	// Notification Service intent and remain idempotent by source event ID.
	"commerce.customer_eta.changed":    {{notifications.EventCommerceCustomerETAChanged, "commerce.customer_eta_changed", notifications.RecipientCustomer, "customer_ref", []string{notifications.ChannelWebSocket, notifications.ChannelInApp}, notifications.PriorityNormal, nil}},
	"commerce.delivery.delay_detected": {{"commerce.customer.delay", "commerce.customer.delay", notifications.RecipientCustomer, "customer_ref", nil, notifications.PriorityHigh, nil}},
}

// NotificationEventTypes returns a stable copy of the controlled outbox
// streams this adapter owns. Keeping the list beside the catalog prevents the
// composition root from silently forgetting a newly supported semantic type.
func NotificationEventTypes() []string {
	result := make([]string, 0, len(commerceNotificationCatalog))
	for eventType := range commerceNotificationCatalog {
		result = append(result, eventType)
	}
	slices.Sort(result)
	return result
}

func (p *NotificationPublisher) Publish(ctx context.Context, record OutboxRecord) error {
	if p == nil {
		return ErrPublisherNotConfigured
	}
	var envelope Envelope
	if err := json.Unmarshal(record.Payload, &envelope); err != nil {
		return fmt.Errorf("notification outbox envelope: %w", err)
	}
	intents := commerceNotificationCatalog[envelope.EventType]
	if len(intents) == 0 {
		return fmt.Errorf("notification event %q is not in the controlled catalog", envelope.EventType)
	}
	var payload map[string]any
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		return fmt.Errorf("notification payload: %w", err)
	}
	for _, intent := range intents {
		if intent.when != nil && !intent.when(payload) {
			continue
		}
		recipient := stringAt(payload, intent.recipientField)
		if recipient == "" && intent.recipientField == "business_ref" {
			recipient = envelope.VendorID
		}
		if recipient == "" {
			return fmt.Errorf("notification %s has no opaque %s", intent.eventType, intent.recipientField)
		}
		data := boundedSemanticData(payload)
		channels := intent.channels
		if len(channels) == 0 {
			channels = notifications.DefaultChannels
		}
		_, err := p.client.Send(ctx, notifications.Request{
			IDempotencyKey:      strings.Join([]string{ServiceName, intent.eventType, envelope.EventID, intent.recipientType, recipient}, ":"),
			SourceService:       ServiceName,
			SourceAggregateType: envelope.AggregateType,
			SourceAggregateID:   envelope.AggregateID,
			SourceEventID:       envelope.EventID,
			SourceVersion:       envelope.AggregateVersion,
			CorrelationRef:      envelope.CorrelationID,
			EventType:           intent.eventType,
			Recipient:           notifications.Recipient{Type: intent.recipientType, ID: recipient},
			Channels:            channels,
			TemplateKey:         intent.template,
			TemplateData:        data,
			Data:                data,
			Priority:            intent.priority,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

var allowedNotificationData = map[string]bool{
	"parent_order_ref": true, "vendor_order_ref": true, "mission_ref": true,
	"mission_slot_ref": true, "eta_version": true, "earliest_at": true,
	"latest_at": true, "confidence": true, "delay_state": true,
	"reason_code": true, "source_version": true, "scheduled_order_ref": true,
	"cutoff_at": true, "deliveries": true, "status": true, "state": true,
	"case_ref": true, "category": true, "resolution": true, "financial_action_ref": true,
}

func boundedSemanticData(payload map[string]any) map[string]any {
	result := map[string]any{}
	for key, value := range payload {
		if allowedNotificationData[key] {
			result[key] = value
		}
	}
	return result
}

func stringAt(payload map[string]any, key string) string {
	value, _ := payload[key].(string)
	return strings.TrimSpace(value)
}

func intAt(payload map[string]any, key string) int64 {
	switch value := payload[key].(type) {
	case float64:
		return int64(value)
	case int64:
		return value
	case json.Number:
		out, _ := value.Int64()
		return out
	case string:
		out, _ := strconv.ParseInt(value, 10, 64)
		return out
	default:
		return 0
	}
}

package events

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cosmicforge/logistics/shared/go/notifications"
)

func TestNotificationPublisherSendsOpaqueCustomerETAReference(t *testing.T) {
	var got notifications.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode notification request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"message_id":"msg_1","status":"accepted"}}`))
	}))
	defer server.Close()

	envelope, err := NewEnvelope("commerce.customer_eta.changed", 1, "VendorOrder", "vo_1", 2, "vendor_1", "corr_1", "", map[string]any{
		"customer_ref":        "cus_opaque",
		"vendor_order_ref":    "vo_1",
		"customer_eta_status": "AVAILABLE",
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(envelope)
	publisher := NewNotificationPublisher(notifications.Client{BaseURL: server.URL, ServiceName: "vendor-service", Secret: []byte("test-secret")})
	if err = publisher.Publish(context.Background(), OutboxRecord{ID: envelope.EventID, EventType: envelope.EventType, AggregateID: envelope.AggregateID, Payload: raw}); err != nil {
		t.Fatal(err)
	}
	if got.Recipient.Type != notifications.RecipientCustomer || got.Recipient.ID != "cus_opaque" || got.EventType != notifications.EventCommerceCustomerETAChanged {
		t.Fatalf("unexpected notification request: %+v", got)
	}
	if got.IDempotencyKey == "" || got.TemplateKey != "commerce.customer_eta_changed" {
		t.Fatalf("missing idempotency/template evidence: %+v", got)
	}
	if got.SourceAggregateType != "VendorOrder" || got.SourceAggregateID != "vo_1" || got.SourceEventID != envelope.EventID || got.SourceVersion != 2 || got.CorrelationRef != "corr_1" {
		t.Fatalf("missing source provenance: %+v", got)
	}
}

func TestNotificationPublisherControlledCommerceMatrix(t *testing.T) {
	tests := []struct {
		name, source, event, template, recipientType, recipientField string
	}{
		{"vendor new order", "commerce.vendor_order.created", "commerce.vendor.new_order", "commerce.vendor.new_order", notifications.RecipientVendor, "business_ref"},
		{"customer accepted", "commerce.vendor_order.accepted", "commerce.customer.order_accepted", "commerce.customer.order_accepted", notifications.RecipientCustomer, "customer_ref"},
		{"customer rejected", "commerce.vendor_order.rejected", "commerce.customer.order_rejected", "commerce.customer.order_rejected", notifications.RecipientCustomer, "customer_ref"},
		{"customer substitution", "commerce.vendor_order.substitution_proposed", "commerce.customer.substitution", "commerce.customer.substitution", notifications.RecipientCustomer, "customer_ref"},
		{"customer ready", "commerce.vendor_order.ready", "commerce.customer.ready", "commerce.customer.ready", notifications.RecipientCustomer, "customer_ref"},
		{"vendor arrived", "commerce.vendor_order.logistics_provider_arrived", "commerce.vendor.provider_arrived", "commerce.vendor.provider_arrived", notifications.RecipientVendor, "business_ref"},
		{"vendor pickup", "commerce.vendor_order.logistics_picked_up", "commerce.vendor.pickup_confirmed", "commerce.vendor.pickup_confirmed", notifications.RecipientVendor, "business_ref"},
		{"scheduled cutoff", "commerce.scheduled_order.cutoff", "commerce.vendor.scheduled_cutoff", "commerce.vendor.scheduled_cutoff", notifications.RecipientVendor, "business_ref"},
		{"delay", "commerce.delivery.delay_detected", "commerce.customer.delay", "commerce.customer.delay", notifications.RecipientCustomer, "customer_ref"},
		{"delivered", "commerce.vendor_order.logistics_delivered", "commerce.customer.delivered", "commerce.customer.delivered", notifications.RecipientCustomer, "customer_ref"},
		{"support", "commerce.vendor.support_action", "commerce.vendor.support_action", "commerce.vendor.support_action", notifications.RecipientVendor, "business_ref"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got notifications.Request
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					t.Fatalf("decode: %v", err)
				}
				_, _ = w.Write([]byte(`{"success":true,"data":{"message_id":"msg","status":"accepted"}}`))
			}))
			defer server.Close()
			payload := map[string]any{tc.recipientField: "opaque_1", "parent_order_ref": "po_1", "password": "must-not-cross"}
			envelope, err := NewEnvelope(tc.source, 1, "VendorOrder", "vo_1", 4, "vendor_1", "correlation_1", "", payload, false)
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(envelope)
			publisher := NewNotificationPublisher(notifications.Client{BaseURL: server.URL, ServiceName: ServiceName, Secret: []byte("secret")})
			if err = publisher.Publish(context.Background(), OutboxRecord{Payload: raw}); err != nil {
				t.Fatal(err)
			}
			if got.EventType != tc.event || got.TemplateKey != tc.template || got.Recipient.Type != tc.recipientType || got.Recipient.ID != "opaque_1" {
				t.Fatalf("unexpected intent: %+v", got)
			}
			if _, leaked := got.Data["password"]; leaked {
				t.Fatalf("unbounded payload crossed notification boundary: %+v", got.Data)
			}
		})
	}
}

func TestNotificationPublisherRejectsUnknownOrRecipientlessIntent(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"success":true,"data":{"message_id":"msg","status":"accepted"}}`))
	}))
	defer server.Close()
	publisher := NewNotificationPublisher(notifications.Client{BaseURL: server.URL, ServiceName: ServiceName, Secret: []byte("secret")})
	for _, tc := range []struct {
		event string
		data  map[string]any
		want  string
	}{
		{"commerce.unknown", map[string]any{"customer_ref": "customer_1"}, "controlled catalog"},
		{"commerce.vendor_order.accepted", map[string]any{}, "no opaque customer_ref"},
	} {
		envelope, err := NewEnvelope(tc.event, 1, "VendorOrder", "vo_1", 2, "vendor_1", "corr", "", tc.data, false)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := json.Marshal(envelope)
		err = publisher.Publish(context.Background(), OutboxRecord{Payload: raw})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("event %s error = %v, want %q", tc.event, err, tc.want)
		}
	}
	if requests != 0 {
		t.Fatalf("invalid intents sent %d notification requests", requests)
	}
}

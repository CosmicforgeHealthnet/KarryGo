package webhooks

import (
	"context"
	"testing"

	"cosmicforge/logistics/services/api-gateway/internal/features/partner/delivery"
)

func strptr(s string) *string { return &s }

// seedBookingAndWebhook wires a partner booking (by dispatch id) plus one active
// webhook subscribed to eventType, with an encrypted secret the service can decrypt.
func (h *webhookHarness) seedBookingAndWebhook(t *testing.T, dispatchID, bookingID, partnerID, eventType string) {
	t.Helper()
	h.book.byDispatchID[dispatchID] = &delivery.PartnerBooking{
		ID: bookingID, PartnerID: partnerID, BookingRef: "KGP-AAAA1111",
		ExternalRef: strptr("ORDER-1"), DispatchBookingID: strptr(dispatchID),
	}
	enc, err := h.crypto.EncryptSecret("whsec_seeded")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	h.repo.hooks["hook-"+eventType] = &Webhook{
		ID: "hook-" + eventType, PartnerID: partnerID, URL: "https://x.com/hook",
		Events: []string{eventType}, EncryptedSigningSecret: enc, Status: StatusActive,
	}
}

func TestProcessDispatchEventMapping(t *testing.T) {
	cases := []struct {
		topic      string
		wantEvent  string
		wantStatus string
	}{
		{DispatchTopicRequestAccepted, EventDeliveryAccepted, delivery.StatusAccepted},
		{DispatchTopicNoProviderFound, EventDeliveryNoProvider, delivery.StatusNoProvider},
		{DispatchTopicTripStarted, EventDeliveryInProgress, delivery.StatusInProgress},
		{DispatchTopicTripCompleted, EventDeliveryCompleted, delivery.StatusCompleted},
		{DispatchTopicTripCancelled, EventDeliveryCancelled, delivery.StatusCancelled},
	}
	for _, tc := range cases {
		t.Run(tc.topic, func(t *testing.T) {
			h := newWebhookHarness(t)
			h.seedBookingAndWebhook(t, "disp-1", "bk1", "p1", tc.wantEvent)
			payload := []byte(`{"event":"` + tc.topic + `","booking_id":"disp-1","occurred_at":"2026-06-28T10:45:00Z"}`)

			if err := h.svc.ProcessDispatchEvent(context.Background(), tc.topic, payload); err != nil {
				t.Fatalf("ProcessDispatchEvent() error = %v", err)
			}

			if got := h.book.statusUpdates["bk1"]; got != tc.wantStatus {
				t.Fatalf("booking status = %q, want %q", got, tc.wantStatus)
			}
			if len(h.drepo.created) != 1 {
				t.Fatalf("delivery records = %d, want 1", len(h.drepo.created))
			}
			d := h.drepo.created[0]
			if d.EventType != tc.wantEvent {
				t.Fatalf("delivery event_type = %q, want %q", d.EventType, tc.wantEvent)
			}
			if d.BookingID != "bk1" {
				t.Fatalf("delivery booking_id = %q, want bk1", d.BookingID)
			}
			if h.sender.calls != 1 {
				t.Fatalf("sender calls = %d, want 1 (immediate attempt)", h.sender.calls)
			}
			if _, ok := h.drepo.delivered[d.ID]; !ok {
				t.Fatal("delivery not marked delivered after 2xx")
			}
		})
	}
}

func TestProcessDispatchEventNoBookingIgnored(t *testing.T) {
	h := newWebhookHarness(t)
	// No booking seeded for disp-x.
	payload := []byte(`{"event":"trip.completed","booking_id":"disp-x"}`)
	if err := h.svc.ProcessDispatchEvent(context.Background(), DispatchTopicTripCompleted, payload); err != nil {
		t.Fatalf("ProcessDispatchEvent() error = %v", err)
	}
	if len(h.drepo.created) != 0 || len(h.book.statusUpdates) != 0 || h.sender.calls != 0 {
		t.Fatal("event with no matching partner booking should be ignored")
	}
}

func TestProcessDispatchEventMalformedIgnored(t *testing.T) {
	h := newWebhookHarness(t)
	if err := h.svc.ProcessDispatchEvent(context.Background(), DispatchTopicTripCompleted, []byte(`{not json`)); err != nil {
		t.Fatalf("malformed payload should be dropped, got error %v", err)
	}
	if len(h.drepo.created) != 0 {
		t.Fatal("malformed event must not create deliveries")
	}
}

func TestProcessDispatchEventUnknownTopicIgnored(t *testing.T) {
	h := newWebhookHarness(t)
	if err := h.svc.ProcessDispatchEvent(context.Background(), "some.unrelated.topic", []byte(`{"booking_id":"disp-1"}`)); err != nil {
		t.Fatalf("unknown topic should be ignored, got %v", err)
	}
	if len(h.drepo.created) != 0 {
		t.Fatal("unknown topic must not create deliveries")
	}
}

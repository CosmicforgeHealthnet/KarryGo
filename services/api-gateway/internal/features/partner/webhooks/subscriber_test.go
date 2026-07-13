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

func TestProcessDispatchEventTripStartedStoresRiderSnapshot(t *testing.T) {
	h := newWebhookHarness(t)
	h.seedBookingAndWebhook(t, "disp-1", "bk1", "p1", EventDeliveryInProgress)
	payload := []byte(`{
		"event": "trip.started",
		"booking_id": "disp-1",
		"provider_id": "11111111-2222-3333-4444-555555555555",
		"rider_name": "Taiwo Adeleke",
		"rider_phone": "+2348033445566",
		"rider_rating": 4.85,
		"vehicle_color": "Red",
		"vehicle_model": "Honda CB125",
		"plate_number": "LAG-234-XY",
		"occurred_at": "2026-07-05T10:08:00Z"
	}`)

	matched, err := h.svc.ProcessDispatchEventResult(context.Background(), DispatchTopicTripStarted, payload)
	if err != nil {
		t.Fatalf("ProcessDispatchEventResult() error = %v", err)
	}
	if !matched {
		t.Fatal("matched = false, want true")
	}
	if got := h.book.statusUpdates["bk1"]; got != delivery.StatusInProgress {
		t.Fatalf("booking status = %q, want in_progress", got)
	}
	snap, ok := h.book.riderUpdates["bk1"]
	if !ok {
		t.Fatal("rider snapshot not stored")
	}
	if snap.RiderID != "11111111-2222-3333-4444-555555555555" || snap.RiderName != "Taiwo Adeleke" ||
		snap.RiderPhone != "+2348033445566" || snap.RiderRating == nil || *snap.RiderRating != 4.85 ||
		snap.VehicleColor != "Red" || snap.VehicleModel != "Honda CB125" || snap.PlateNumber != "LAG-234-XY" {
		t.Fatalf("rider snapshot = %+v", snap)
	}
	// Webhook still enqueued with the rider data in the payload.
	if len(h.drepo.created) != 1 {
		t.Fatalf("delivery records = %d, want 1", len(h.drepo.created))
	}
	if h.sender.lastPayload.Data["rider"] == nil {
		t.Fatalf("webhook payload data missing rider block: %+v", h.sender.lastPayload.Data)
	}
}

func TestProcessDispatchEventProviderArrivedUpdatesStatusOnly(t *testing.T) {
	h := newWebhookHarness(t)
	h.book.byDispatchID["disp-1"] = &delivery.PartnerBooking{
		ID: "bk1", PartnerID: "p1", BookingRef: "KGP-AAAA1111",
		ExternalRef: strptr("ORDER-1"), DispatchBookingID: strptr("disp-1"),
	}
	payload := []byte(`{"event":"trip.provider_arrived","booking_id":"disp-1","provider_id":"11111111-2222-3333-4444-555555555555","occurred_at":"2026-07-05T10:06:00Z"}`)

	matched, err := h.svc.ProcessDispatchEventResult(context.Background(), DispatchTopicProviderArrived, payload)
	if err != nil {
		t.Fatalf("ProcessDispatchEventResult() error = %v", err)
	}
	if !matched {
		t.Fatal("matched = false, want true")
	}
	if got := h.book.statusUpdates["bk1"]; got != delivery.StatusArrived {
		t.Fatalf("booking status = %q, want arrived", got)
	}
	if len(h.drepo.created) != 0 || h.sender.calls != 0 {
		t.Fatalf("provider-arrived must be status-only until webhook event is supported: deliveries=%d sends=%d", len(h.drepo.created), h.sender.calls)
	}
}
func TestProcessDispatchEventResultUnmatchedReturnsFalse(t *testing.T) {
	h := newWebhookHarness(t)
	matched, err := h.svc.ProcessDispatchEventResult(context.Background(), DispatchTopicTripCompleted,
		[]byte(`{"event":"trip.completed","booking_id":"disp-unknown"}`))
	if err != nil {
		t.Fatalf("ProcessDispatchEventResult() error = %v", err)
	}
	if matched {
		t.Fatal("matched = true for unknown booking, want false")
	}
}

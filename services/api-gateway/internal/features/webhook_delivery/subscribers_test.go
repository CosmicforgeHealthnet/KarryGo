package webhookdelivery

import (
	"context"
	"testing"
)

func TestTopicToWebhookEvent(t *testing.T) {
	cases := map[string]string{
		"request.accepted":          "delivery.accepted",
		"request.no_provider_found": "delivery.no_provider",
		"trip.started":              "delivery.in_progress",
		"trip.completed":            "delivery.completed",
		"trip.cancelled":            "delivery.cancelled",
		"trip.failed":               "delivery.failed",
		"unknown.topic":             "",
	}
	for topic, want := range cases {
		if got := topicToWebhookEvent(topic); got != want {
			t.Fatalf("topicToWebhookEvent(%q) = %q, want %q", topic, got, want)
		}
	}
}

// TestHandleEventMapping covers all 6 dispatch topics: status update + enqueue.
func TestHandleEventMapping(t *testing.T) {
	cases := []struct {
		topic      string
		status     string
		wantEvent  string
		wantStatus string
	}{
		{"request.accepted", "accepted", "delivery.accepted", "accepted"},
		{"request.no_provider_found", "no_provider", "delivery.no_provider", "no_provider"},
		{"trip.started", "in_progress", "delivery.in_progress", "in_progress"},
		{"trip.completed", "completed", "delivery.completed", "completed"},
		{"trip.cancelled", "cancelled", "delivery.cancelled", "cancelled"},
		{"trip.failed", "failed", "delivery.failed", "failed"},
	}
	for _, tc := range cases {
		t.Run(tc.topic, func(t *testing.T) {
			repo := newFakeRepo()
			repo.booking = &Booking{ID: "bk1", PartnerID: "p1", DispatchBookingID: "disp1"}
			worker := &fakeWorker{}
			sub := NewDispatchSubscriber(nil, repo, worker)

			err := sub.handleEvent(context.Background(), tc.topic, tc.status, []byte(`{"booking_id":"disp1"}`))
			if err != nil {
				t.Fatalf("handleEvent error = %v", err)
			}
			if repo.statusUpdates["bk1"] != tc.wantStatus {
				t.Fatalf("status = %q, want %q", repo.statusUpdates["bk1"], tc.wantStatus)
			}
			if len(worker.calls) != 1 {
				t.Fatalf("enqueue calls = %d, want 1", len(worker.calls))
			}
			c := worker.calls[0]
			if c.eventType != tc.wantEvent || c.bookingID != "bk1" || c.partnerID != "p1" {
				t.Fatalf("enqueue = %+v, want event %q booking bk1 partner p1", c, tc.wantEvent)
			}
		})
	}
}

func TestHandleEventMalformedJSON(t *testing.T) {
	repo := newFakeRepo()
	repo.booking = &Booking{ID: "bk1", PartnerID: "p1"}
	worker := &fakeWorker{}
	sub := NewDispatchSubscriber(nil, repo, worker)

	if err := sub.handleEvent(context.Background(), "trip.completed", "completed", []byte(`{not json`)); err != nil {
		t.Fatalf("malformed payload should return nil, got %v", err)
	}
	if len(repo.statusUpdates) != 0 || len(worker.calls) != 0 {
		t.Fatal("malformed payload must not update status or enqueue")
	}
}

func TestHandleEventMissingBookingID(t *testing.T) {
	repo := newFakeRepo()
	repo.booking = &Booking{ID: "bk1", PartnerID: "p1"}
	worker := &fakeWorker{}
	sub := NewDispatchSubscriber(nil, repo, worker)

	if err := sub.handleEvent(context.Background(), "trip.completed", "completed", []byte(`{"foo":"bar"}`)); err != nil {
		t.Fatalf("missing booking_id should return nil, got %v", err)
	}
	if len(repo.statusUpdates) != 0 || len(worker.calls) != 0 {
		t.Fatal("missing booking_id must not update status or enqueue")
	}
}

func TestHandleEventNonPartnerBookingSkipped(t *testing.T) {
	repo := newFakeRepo() // booking is nil -> not found
	worker := &fakeWorker{}
	sub := NewDispatchSubscriber(nil, repo, worker)

	if err := sub.handleEvent(context.Background(), "trip.completed", "completed", []byte(`{"booking_id":"disp-x"}`)); err != nil {
		t.Fatalf("non-partner booking should return nil, got %v", err)
	}
	if len(worker.calls) != 0 {
		t.Fatal("non-partner booking must not enqueue")
	}
}

func TestHandleEventStatusUpdateFailure(t *testing.T) {
	repo := newFakeRepo()
	repo.booking = &Booking{ID: "bk1", PartnerID: "p1"}
	repo.updateStatusErr = errBoom
	worker := &fakeWorker{}
	sub := NewDispatchSubscriber(nil, repo, worker)

	err := sub.handleEvent(context.Background(), "trip.completed", "completed", []byte(`{"booking_id":"disp1"}`))
	if err == nil {
		t.Fatal("status update failure should return an error")
	}
	if len(worker.calls) != 0 {
		t.Fatal("must not enqueue when status update fails")
	}
}

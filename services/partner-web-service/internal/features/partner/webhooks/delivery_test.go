package webhooks

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func (h *webhookHarness) encWebhook(t *testing.T, id string) Webhook {
	t.Helper()
	enc, err := h.crypto.EncryptSecret("whsec_x")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	return Webhook{ID: id, PartnerID: "p1", URL: "https://x.com/hook", EncryptedSigningSecret: enc, Status: StatusActive}
}

func TestDeliverOnceSuccessMarksDelivered(t *testing.T) {
	h := newWebhookHarness(t)
	h.sender.status = 200
	status, err := h.svc.deliverOnce(context.Background(), h.encWebhook(t, "w1"), WebhookDelivery{ID: "d1"}, WebhookPayload{Event: "e"})
	if err != nil || status != 200 {
		t.Fatalf("deliverOnce = %d, %v; want 200, nil", status, err)
	}
	if h.drepo.delivered["d1"] != 200 {
		t.Fatal("delivery not marked delivered on 2xx")
	}
}

func TestDeliverOnceFailureSchedulesRetry(t *testing.T) {
	h := newWebhookHarness(t)
	h.sender.status = 500
	h.sender.err = errors.New("server error")

	_, err := h.svc.deliverOnce(context.Background(), h.encWebhook(t, "w1"), WebhookDelivery{ID: "d1", Attempts: 0}, WebhookPayload{Event: "e"})
	if err == nil {
		t.Fatal("deliverOnce err = nil, want failure")
	}
	if h.drepo.failedAttempt["d1"] != 1 {
		t.Fatalf("attempts recorded = %d, want 1", h.drepo.failedAttempt["d1"])
	}
	if h.drepo.failedAttemptTo["d1"] == nil {
		t.Fatal("next_attempt_at not scheduled")
	}
	if h.drepo.failedPermanent["d1"] {
		t.Fatal("marked failed_permanent too early")
	}
}

func TestDeliverOnceExhaustsToFailedPermanent(t *testing.T) {
	h := newWebhookHarness(t)
	h.sender.status = 500
	h.sender.err = errors.New("server error")

	// Attempts already at 4 -> this is the 5th attempt -> no more retries.
	_, err := h.svc.deliverOnce(context.Background(), h.encWebhook(t, "w1"), WebhookDelivery{ID: "d1", Attempts: 4}, WebhookPayload{Event: "e"})
	if err == nil {
		t.Fatal("deliverOnce err = nil, want failure")
	}
	if !h.drepo.failedPermanent["d1"] {
		t.Fatal("delivery not marked failed_permanent after 5 attempts")
	}
}

func TestProcessDueDeliveriesRetriesAndDelivers(t *testing.T) {
	h := newWebhookHarness(t)
	h.sender.status = 200
	w := h.encWebhook(t, "w1")
	h.repo.hooks["w1"] = &w
	body, _ := json.Marshal(WebhookPayload{Event: EventDeliveryCompleted, DeliveryID: "bk1"})
	h.drepo.due = []WebhookDelivery{{ID: "d1", WebhookID: "w1", Payload: body, Attempts: 1, Status: DeliveryStatusPending}}

	if err := h.svc.ProcessDueDeliveries(context.Background(), 50); err != nil {
		t.Fatalf("ProcessDueDeliveries() error = %v", err)
	}
	if h.drepo.delivered["d1"] != 200 {
		t.Fatal("due delivery not delivered on retry")
	}
}

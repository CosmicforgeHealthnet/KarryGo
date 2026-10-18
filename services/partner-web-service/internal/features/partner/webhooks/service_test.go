package webhooks

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"cosmicforge/logistics/services/partner-web-service/internal/features/partner/delivery"
)

func seqIDs() func() string {
	var n int
	var mu sync.Mutex
	return func() string {
		mu.Lock()
		defer mu.Unlock()
		n++
		return "wid-" + strconv.Itoa(n)
	}
}

type webhookHarness struct {
	svc    *Service
	repo   *fakeWebhookRepo
	drepo  *fakeDeliveryRepo
	book   *fakeBookingLookup
	sender *fakeSender
	crypto *Cryptor
}

func newWebhookHarness(t *testing.T) *webhookHarness {
	t.Helper()
	h := &webhookHarness{
		repo:   newFakeWebhookRepo(),
		drepo:  newFakeDeliveryRepo(),
		book:   newFakeBookingLookup(),
		sender: &fakeSender{status: 200},
		crypto: testCryptor(t),
	}
	h.svc = NewService(Deps{
		Webhooks:   h.repo,
		Deliveries: h.drepo,
		Bookings:   h.book,
		Cryptor:    h.crypto,
		Sender:     h.sender,
		Now:        func() time.Time { return time.Date(2026, 6, 28, 10, 0, 0, 0, time.UTC) },
		NewID:      seqIDs(),
	})
	return h
}

func wantWebhookErr(t *testing.T, err error, code string, status int) {
	t.Helper()
	var e *delivery.APIError
	if !errors.As(err, &e) {
		t.Fatalf("error = %v, want *delivery.APIError", err)
	}
	if e.Code != code || e.Status != status {
		t.Fatalf("error = %s/%d, want %s/%d", e.Code, e.Status, code, status)
	}
}

func TestCreateWebhookReturnsSecretOnceAndStoresEncrypted(t *testing.T) {
	h := newWebhookHarness(t)
	res, err := h.svc.CreateWebhook(context.Background(), "p1", CreateWebhookInput{
		URL:    "https://pharmacy.com/karrygo-webhook",
		Events: []string{EventDeliveryAccepted, EventDeliveryCompleted},
	})
	if err != nil {
		t.Fatalf("CreateWebhook() error = %v", err)
	}
	if !strings.HasPrefix(res.SigningSecret, "whsec_") {
		t.Fatalf("signing_secret = %q, want whsec_ prefix", res.SigningSecret)
	}
	if res.Status != StatusActive {
		t.Fatalf("status = %q, want active", res.Status)
	}

	stored := h.repo.hooks[res.ID]
	if stored == nil {
		t.Fatal("webhook not stored")
	}
	if stored.EncryptedSigningSecret == res.SigningSecret {
		t.Fatal("secret stored in plaintext")
	}
	if strings.Contains(stored.EncryptedSigningSecret, res.SigningSecret) {
		t.Fatal("plaintext secret leaked into stored ciphertext")
	}
	decrypted, err := h.crypto.DecryptSecret(stored.EncryptedSigningSecret)
	if err != nil {
		t.Fatalf("decrypt stored secret: %v", err)
	}
	if decrypted != res.SigningSecret {
		t.Fatal("stored secret does not decrypt back to the issued secret")
	}
}

func TestCreateWebhookValidationRejections(t *testing.T) {
	h := newWebhookHarness(t)
	cases := []struct {
		name   string
		url    string
		events []string
	}{
		{"http rejected", "http://pharmacy.com/hook", []string{EventDeliveryAccepted}},
		{"localhost rejected", "https://localhost/hook", []string{EventDeliveryAccepted}},
		{"private IP rejected", "https://10.0.0.5/hook", []string{EventDeliveryAccepted}},
		{"invalid event rejected", "https://pharmacy.com/hook", []string{"delivery.bogus"}},
		{"empty events rejected", "https://pharmacy.com/hook", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.svc.CreateWebhook(context.Background(), "p1", CreateWebhookInput{URL: tc.url, Events: tc.events})
			wantWebhookErr(t, err, "validation_error", 400)
		})
	}
}

func TestListWebhooksNeverReturnsSecret(t *testing.T) {
	h := newWebhookHarness(t)
	res, _ := h.svc.CreateWebhook(context.Background(), "p1", CreateWebhookInput{
		URL: "https://pharmacy.com/hook", Events: []string{EventDeliveryAccepted},
	})
	views, err := h.svc.ListWebhooks(context.Background(), "p1")
	if err != nil {
		t.Fatalf("ListWebhooks() error = %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("len(views) = %d, want 1", len(views))
	}
	blob, _ := json.Marshal(views)
	if strings.Contains(string(blob), res.SigningSecret) || strings.Contains(string(blob), "signing_secret") {
		t.Fatalf("GET /webhooks leaked the signing secret: %s", blob)
	}
}

func TestDeleteWebhookIsPartnerScoped(t *testing.T) {
	h := newWebhookHarness(t)
	res, _ := h.svc.CreateWebhook(context.Background(), "p1", CreateWebhookInput{
		URL: "https://pharmacy.com/hook", Events: []string{EventDeliveryAccepted},
	})

	// Another partner cannot delete it.
	if _, err := h.svc.DeleteWebhook(context.Background(), "p2", res.ID); err == nil {
		t.Fatal("delete by other partner succeeded, want webhook_not_found")
	} else {
		wantWebhookErr(t, err, "webhook_not_found", 404)
	}
	if h.repo.hooks[res.ID].Status != StatusActive {
		t.Fatal("webhook was modified by a non-owner delete")
	}

	// Owner can delete (soft delete -> inactive).
	del, err := h.svc.DeleteWebhook(context.Background(), "p1", res.ID)
	if err != nil {
		t.Fatalf("DeleteWebhook() error = %v", err)
	}
	if !del.Deleted {
		t.Fatal("Deleted = false, want true")
	}
	if h.repo.hooks[res.ID].Status != StatusInactive {
		t.Fatalf("status = %q, want inactive", h.repo.hooks[res.ID].Status)
	}
}

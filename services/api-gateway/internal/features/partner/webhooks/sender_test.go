package webhooks

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestHTTPSenderSendsSignedHeaders(t *testing.T) {
	const secret = "whsec_testsecret"
	var (
		gotSig, gotEvent, gotDelivery, gotTS, gotCT string
		gotBody                                     []byte
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotSig = r.Header.Get(HeaderSignature)
		gotEvent = r.Header.Get(HeaderEvent)
		gotDelivery = r.Header.Get(HeaderDelivery)
		gotTS = r.Header.Get(HeaderTimestamp)
		gotCT = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	sender := NewHTTPSender()
	now := time.Unix(1717581600, 0)
	payload := WebhookPayload{Event: EventDeliveryCompleted, DeliveryID: "b1", BookingRef: "KGP-AAAA1111", Status: "completed", Data: map[string]any{}}
	status, err := sender.Send(context.Background(), Webhook{URL: srv.URL}, WebhookDelivery{ID: "d1"}, secret, payload, now)
	if err != nil || status != http.StatusOK {
		t.Fatalf("Send() = %d, %v; want 200, nil", status, err)
	}

	if gotCT != "application/json" {
		t.Fatalf("Content-Type = %q", gotCT)
	}
	if gotEvent != EventDeliveryCompleted {
		t.Fatalf("X-KarryGo-Event = %q", gotEvent)
	}
	if gotDelivery != "d1" {
		t.Fatalf("X-KarryGo-Delivery = %q", gotDelivery)
	}
	if gotTS != strconv.FormatInt(now.UTC().Unix(), 10) {
		t.Fatalf("X-KarryGo-Timestamp = %q", gotTS)
	}
	// Signature must match HMAC over the exact received body + timestamp.
	want := SignPayload(secret, now.UTC().Unix(), gotBody)
	if gotSig != want {
		t.Fatalf("X-KarryGo-Signature = %q, want %q", gotSig, want)
	}
}

func TestHTTPSender500IsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	status, err := NewHTTPSender().Send(context.Background(), Webhook{URL: srv.URL}, WebhookDelivery{ID: "d1"}, "s", WebhookPayload{Event: "e"}, time.Now())
	if err == nil {
		t.Fatal("Send() err = nil, want failure on 500")
	}
	if status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", status)
	}
}

func TestHTTPSenderNetworkErrorIsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // now nothing is listening

	status, err := NewHTTPSender().Send(context.Background(), Webhook{URL: url}, WebhookDelivery{ID: "d1"}, "s", WebhookPayload{Event: "e"}, time.Now())
	if err == nil {
		t.Fatal("Send() err = nil, want network error")
	}
	if status != 0 {
		t.Fatalf("status = %d, want 0 on network error", status)
	}
}

func TestServiceTestWebhookSendsSignedPayload(t *testing.T) {
	const secret = "whsec_realsecret1234567890abcdef"
	cryptor := testCryptor(t)
	enc, err := cryptor.EncryptSecret(secret)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	var validSig bool
	var gotEvent string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		ts, _ := strconv.ParseInt(r.Header.Get(HeaderTimestamp), 10, 64)
		validSig = r.Header.Get(HeaderSignature) == SignPayload(secret, ts, body)
		gotEvent = r.Header.Get(HeaderEvent)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	repo := newFakeWebhookRepo()
	repo.hooks["wid-1"] = &Webhook{ID: "wid-1", PartnerID: "p1", URL: srv.URL, Events: []string{EventDeliveryAccepted}, EncryptedSigningSecret: enc, Status: StatusActive}
	svc := NewService(Deps{Webhooks: repo, Cryptor: cryptor, Sender: NewHTTPSender()})

	if err := svc.TestWebhook(context.Background(), "p1", "wid-1"); err != nil {
		t.Fatalf("TestWebhook() error = %v", err)
	}
	if !validSig {
		t.Fatal("test webhook signature did not validate on the receiver")
	}
	if gotEvent != EventDeliveryTest {
		t.Fatalf("event = %q, want %s", gotEvent, EventDeliveryTest)
	}
}

func TestServiceTestWebhookFailureReturns502(t *testing.T) {
	const secret = "whsec_x"
	cryptor := testCryptor(t)
	enc, _ := cryptor.EncryptSecret(secret)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	repo := newFakeWebhookRepo()
	repo.hooks["wid-1"] = &Webhook{ID: "wid-1", PartnerID: "p1", URL: srv.URL, EncryptedSigningSecret: enc, Status: StatusActive}
	svc := NewService(Deps{Webhooks: repo, Cryptor: cryptor, Sender: NewHTTPSender()})

	err := svc.TestWebhook(context.Background(), "p1", "wid-1")
	wantWebhookErr(t, err, "webhook_delivery_failed", 502)
}

func TestServiceTestWebhookNotFound(t *testing.T) {
	svc := NewService(Deps{Webhooks: newFakeWebhookRepo(), Cryptor: testCryptor(t), Sender: &fakeSender{status: 200}})
	err := svc.TestWebhook(context.Background(), "p1", "missing")
	wantWebhookErr(t, err, "webhook_not_found", 404)
}

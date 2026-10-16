package authclients

import (
	"context"
	"cosmicforge/logistics/shared/go/notifications"
	"cosmicforge/logistics/shared/go/serviceauth"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDispatchEmailUsesAuthenticatedNotificationService(t *testing.T) {
	secret := []byte("test-notification-secret")
	verifier := serviceauth.NewVerifier(serviceauth.Secrets{"driver-dispatch-delivery-service": secret}, time.Minute)
	var requests []notifications.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := verifier.VerifyRequest(r); err != nil {
			t.Error(err)
			w.WriteHeader(401)
			return
		}
		if r.URL.Path != "/api/v1/notifications/send" {
			t.Error("wrong notification route")
		}
		var request notifications.Request
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
		}
		requests = append(requests, request)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(202)
		_, _ = w.Write([]byte(`{"success":true,"data":{"message_id":"message","status":"QUEUED"}}`))
	}))
	defer server.Close()
	client := NewNotificationEmailClient(notifications.Client{BaseURL: server.URL, ServiceName: "driver-dispatch-delivery-service", Secret: secret, HTTPClient: server.Client()})
	for _, code := range []string{"123456", "654321"} {
		if err := client.SendOTP(context.Background(), "rider@example.com", code); err != nil {
			t.Fatal(err)
		}
	}
	if len(requests) != 2 || requests[0].IDempotencyKey == requests[1].IDempotencyKey {
		t.Fatal("OTP resend was deduplicated")
	}
	if len(requests[0].Channels) != 1 || requests[0].Channels[0] != notifications.ChannelEmail || !strings.Contains(requests[1].Body, "654321") {
		t.Fatal("wrong OTP payload")
	}
	if err := NewNotificationEmailClient(nil).SendOTP(context.Background(), "rider@example.com", "123456"); !errors.Is(err, ErrEmailNotConfigured) {
		t.Fatalf("unconfigured error: %v", err)
	}
}

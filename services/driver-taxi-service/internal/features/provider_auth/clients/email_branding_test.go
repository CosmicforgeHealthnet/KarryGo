package providerauthclients

import (
	"context"
	"cosmicforge/logistics/shared/go/notifications"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmailOTPBrandingPreservesCodeAndContract(t *testing.T) {
	var captured notifications.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/notifications/send" {
			t.Error("unexpected notification route")
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"message_id":"fixture-message","status":"queued"}}`))
	}))
	defer server.Close()
	client := notifications.Client{BaseURL: server.URL, ServiceName: "driver-taxi-service", Secret: []byte("test-secret")}
	if err := NewEmailOTPSender(client).SendLoginCode(context.Background(), "email", "fixture@example.com", "012345"); err != nil {
		t.Fatal(err)
	}
	if captured.Title != "Your Karryam driver sign-in code" || captured.Body != "Your sign-in code is 012345. It expires in a few minutes. If you did not ask for it, ignore this email." {
		t.Fatal("email branding, code or security copy changed")
	}
	if captured.SourceService != "driver-taxi-service" || captured.EventType != "provider.auth.otp" || len(captured.Channels) != 1 || captured.Channels[0] != notifications.ChannelEmail {
		t.Fatal("notification contract changed")
	}
}

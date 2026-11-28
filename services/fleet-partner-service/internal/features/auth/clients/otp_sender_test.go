package clients

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/notifications"
)

func TestCorporateOTPHasRenderableBodyWithoutSeededTemplate(t *testing.T) {
	const code = "052814"
	var captured notifications.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/notifications/send" {
			t.Errorf("unexpected notification route %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected route", http.StatusNotFound)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Errorf("decode notification: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"message_id":"message-1","status":"queued"}}`))
	}))
	defer server.Close()

	sender := NewNotificationSender(server.URL, []byte("test-signing-secret"), time.Second)
	reference, err := sender.SendOTP(context.Background(), OTPMessage{
		Email: "owner@example.com", Phone: "+2348012345678", Code: code,
		Purpose: "SIGN_UP_OR_LOGIN", ExpiresIn: 10 * time.Minute,
		IdempotencyKey: "otp:challenge-1",
	})
	if err != nil || reference != "message-1" {
		t.Fatalf("SendOTP() = (%q, %v)", reference, err)
	}
	if captured.TemplateKey != "corporate.auth.otp" || captured.EventType != EventCorporateAuthOTP {
		t.Fatalf("notification template/event mismatch: %q / %q", captured.TemplateKey, captured.EventType)
	}
	if captured.Title != "Your Karryam verification code" ||
		!strings.Contains(captured.Body, code) ||
		!strings.Contains(captured.Body, "10 minutes") ||
		len(captured.Channels) != 1 || captured.Channels[0] != notifications.ChannelEmail {
		t.Fatal("OTP delivery must have a code-bearing email body even without a seeded template")
	}
}

// Staff and provider invitations were sent with a subject only and a template
// key that was never provisioned; notification-service refuses an email
// without a body, so no invitation was ever delivered. The body must always
// carry the acceptance link, and a caller-supplied body must still win.
func TestInvitationHasRenderableBodyWithoutSeededTemplate(t *testing.T) {
	var captured notifications.Request
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured = notifications.Request{}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"message_id":"message-2","status":"queued"}}`))
	}))
	defer server.Close()
	sender := NewNotificationSender(server.URL, []byte("test-signing-secret"), time.Second)

	const acceptURL = "https://fleet.example.test/sign-up?token=opaque-token"
	expires := time.Date(2026, 12, 5, 9, 19, 0, 0, time.UTC)
	if _, err := sender.SendInvitation(context.Background(), InvitationMessage{
		Email: "staff@example.com", OrganizationName: "Cosmicforge", AcceptURL: acceptURL,
		ExpiresAt: expires, EventType: EventCorporateStaffInvitation, IdempotencyKey: "staff-invite:inv_1",
	}); err != nil {
		t.Fatalf("SendInvitation() error = %v", err)
	}
	if !strings.Contains(captured.Body, acceptURL) || !strings.Contains(captured.Body, "Cosmicforge") ||
		!strings.Contains(captured.Body, "5 December 2026") || strings.Contains(strings.ToLower(captured.Body), "password:") {
		t.Fatalf("invitation body missing link, organization or expiry: %q", captured.Body)
	}
	if captured.Title != "You have been invited to Cosmicforge" || len(captured.Channels) != 1 || captured.Channels[0] != notifications.ChannelEmail {
		t.Fatalf("invitation title/channels mismatch: %q %v", captured.Title, captured.Channels)
	}

	if _, err := sender.SendInvitation(context.Background(), InvitationMessage{
		Email: "provider@example.com", OrganizationName: "Cosmicforge", Title: "Finish setting up",
		Body: "Open your provider app.", EventType: EventCorporateProviderLoginNotice, IdempotencyKey: "notice:1",
	}); err != nil {
		t.Fatalf("SendInvitation(notice) error = %v", err)
	}
	if captured.Body != "Open your provider app." {
		t.Fatalf("caller-supplied body must be kept, got %q", captured.Body)
	}
}

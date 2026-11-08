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
    Purpose: "SIGN_UP_OR_LOGIN", ExpiresIn: 10*time.Minute,
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

package authclients

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestAuthEventTopicConstants(t *testing.T) {
	cases := map[string]string{
		"otp requested":     TopicOTPRequested,
		"session created":   TopicSessionCreated,
		"logged out":        TopicLoggedOut,
		"profile suspended": TopicProfileSuspended,
	}

	want := map[string]string{
		"otp requested":     "provider.auth.otp_requested",
		"session created":   "provider.auth.session_created",
		"logged out":        "provider.auth.logged_out",
		"profile suspended": "provider.profile.suspended",
	}

	for name, got := range cases {
		if got != want[name] {
			t.Fatalf("%s topic = %q, want %q", name, got, want[name])
		}
	}
}

func TestAuthEventPayloadsIncludeCorrelationIDAndCreatedAt(t *testing.T) {
	createdAt := time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)

	payloads := []struct {
		name  string
		value any
	}{
		{
			name: "otp requested",
			value: OTPRequestedEvent{
				Event:         TopicOTPRequested,
				CorrelationID: "req-otp",
				PhoneNumber:   "+2348012345678",
				OTPCode:       "123456",
				Purpose:       "login",
				ExpiresIn:     600,
				CreatedAt:     createdAt,
			},
		},
		{
			name: "session created",
			value: SessionCreatedEvent{
				Event:         TopicSessionCreated,
				CorrelationID: "req-session",
				ProviderID:    "provider-1",
				PhoneNumber:   "+2348012345678",
				Role:          "dispatch_provider",
				SessionID:     "session-1",
				CreatedAt:     createdAt,
			},
		},
		{
			name: "logged out",
			value: LoggedOutEvent{
				Event:         TopicLoggedOut,
				CorrelationID: "req-logout",
				ProviderID:    "provider-1",
				SessionID:     "session-1",
				CreatedAt:     createdAt,
			},
		},
	}

	for _, tc := range payloads {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var decoded map[string]any
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if decoded["correlation_id"] == "" {
				t.Fatal("correlation_id must be present")
			}
			if decoded["created_at"] == "" {
				t.Fatal("created_at must be present")
			}
			lower := strings.ToLower(string(raw))
			if strings.Contains(lower, "access_token") || strings.Contains(lower, "refresh_token") {
				t.Fatalf("event payload must not contain token fields: %s", raw)
			}
		})
	}
}

func TestLoggedOutEventPayloadContract(t *testing.T) {
	createdAt := time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)
	event := LoggedOutEvent{
		Event:         TopicLoggedOut,
		CorrelationID: "req-logout",
		ProviderID:    "provider-1",
		SessionID:     "session-1",
		CreatedAt:     createdAt,
	}

	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, field := range []string{"event", "correlation_id", "provider_id", "session_id", "created_at"} {
		if decoded[field] == "" {
			t.Fatalf("field %q must be present", field)
		}
	}
	if decoded["event"] != TopicLoggedOut {
		t.Fatalf("event = %v, want %s", decoded["event"], TopicLoggedOut)
	}
}

func TestSubscribeProfileSuspendedPlaceholder(t *testing.T) {
	SubscribeProfileSuspended(context.Background(), nil)
}

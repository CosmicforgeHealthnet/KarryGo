package marketplace

import "testing"

func TestGenericEventPayloadRejectsNestedHandoverSecrets(t *testing.T) {
	for _, payload := range []map[string]any{
		{"otp": "123456"},
		{"nested": map[string]any{"qr_token": "secret"}},
		{"nested": []map[string]string{{"OTP_HASH": "secret"}}},
		{"items": []any{map[string]any{"otpHash": "hash"}}},
		{"receiver_email": "private@example.test"},
	} {
		if err := validatePayload(EventTripStatusChanged, payload); err == nil {
			t.Fatalf("secret payload accepted: %#v", payload)
		}
	}
	if err := validatePayload(EventTripStatusChanged, map[string]any{"status": "in_progress", "challengeRef": "opaque-ref"}); err != nil {
		t.Fatalf("safe payload rejected: %v", err)
	}
}

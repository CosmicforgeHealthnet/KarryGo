package events

import (
	"errors"
	"strings"
	"testing"
)

func TestRedactErrorRemovesCredentials(t *testing.T) {
	raw := `request failed authorization: Bearer ey.secret.token otp=482911 qr=opaque-secret cvv: 123 password="hunter2" access_token=access-secret payment_credential=payment-secret bank_credential=bank-secret card_pan=4111111111111111 hmac_secret=topsecret`
	redacted := RedactError(errors.New(raw))
	for _, secret := range []string{"ey.secret.token", "482911", "opaque-secret", "123", "hunter2", "access-secret", "payment-secret", "bank-secret", "4111111111111111", "topsecret"} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("redacted error leaked %q: %s", secret, redacted)
		}
	}
	if !strings.Contains(redacted, "request failed") {
		t.Fatalf("safe diagnostic context was lost: %s", redacted)
	}
}

func TestRedactErrorIsBounded(t *testing.T) {
	if got := RedactError(errors.New(strings.Repeat("x", 600))); len(got) != 300 {
		t.Fatalf("length=%d want 300", len(got))
	}
}

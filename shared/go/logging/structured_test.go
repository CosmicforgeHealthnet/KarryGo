package logging

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestSafeFieldsRedactsNestedSecretsAndOpaqueObjects(t *testing.T) {
	fields := map[string]any{"trip_id": "trip-a", "source_ref": "source-a", "otp": "SECRET_OTP", "nested": map[string]any{"qr_token": "SECRET_QR", "receiver_phone": "SECRET_PHONE", "attempt": int64(2)}, "error": errors.New("SECRET_ERROR"), "opaque": struct{ Value string }{"SECRET_STRUCT"}}
	encoded, err := json.Marshal(SafeFields(fields))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "SECRET_") {
		t.Fatal("secret not redacted")
	}
	if !strings.Contains(string(encoded), "trip-a") || !strings.Contains(string(encoded), "source-a") {
		t.Fatal("safe correlation lost")
	}
}

// Customer V4 §25.11: settlement account values are redacted even when the
// key does not say "bank"; ordinary account ids are not over-redacted.
func TestSafeFieldsRedactsAccountNumbersButNotAccountIDs(t *testing.T) {
	out := SafeFields(map[string]any{"account_number": "0123456789", "recipient_nuban": "0123456789", "bvn": "22222222222", "account_id": "acct-1"})
	for _, key := range []string{"account_number", "recipient_nuban", "bvn"} {
		if out[key] != "[REDACTED]" {
			t.Errorf("%s not redacted: %v", key, out[key])
		}
	}
	if out["account_id"] != "acct-1" {
		t.Errorf("account_id over-redacted: %v", out["account_id"])
	}
}

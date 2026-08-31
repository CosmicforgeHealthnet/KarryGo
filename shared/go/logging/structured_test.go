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

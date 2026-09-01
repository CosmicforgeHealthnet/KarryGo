package securehandover

import (
	"cosmicforge/logistics/shared/go/handover"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestLegacyCompatibilityStoresOpaqueReferenceNotOTP(t *testing.T) {
	secret := []byte("compatibility-test-handover-key")
	challenge := handover.BuildChallengeID("dispatch", handover.StepPickup, "trip-a")
	record, err := NewCompatibilityCodeRecord(secret, challenge, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	code, err := CompatibilityCode(secret, challenge, record.Code)
	if err != nil {
		t.Fatal(err)
	}
	if record.Code == code || !strings.HasPrefix(record.Code, "handover_") || len(code) != 6 {
		t.Fatal("plaintext storage or invalid public code")
	}
	if !handover.VerifyCode(secret, challenge, code, record.Hash) {
		t.Fatal("legacy verification compatibility broken")
	}
	if handover.VerifyCode(secret, challenge, record.Code, record.Hash) {
		t.Fatal("reference treated as handover authentication")
	}
	for _, other := range []string{"trip-b", "delivery:trip-a"} {
		foreign, _ := CompatibilityCode(secret, other, record.Code)
		if handover.VerifyCode(secret, challenge, foreign, record.Hash) {
			t.Fatal("context separation failed")
		}
	}
	second, _ := NewCompatibilityCodeRecord(secret, challenge, time.Now())
	if second.Code == record.Code {
		t.Fatal("reference reused")
	}
}

func TestSecretTransportIntentJSONExcludesCredentials(t *testing.T) {
	data, err := json.Marshal(SecretDeliveryIntent{OTP: "SECRET_OTP", QRToken: "SECRET_QR"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "SECRET_") {
		t.Fatal("transport intent leaked credentials")
	}
}

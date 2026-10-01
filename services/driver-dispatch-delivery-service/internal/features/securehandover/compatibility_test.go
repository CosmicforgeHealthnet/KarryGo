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

// GAP-C retirement trigger, pinned.
//
// CompatibilityCode returns a stored value verbatim when it lacks the
// "handover_" prefix. That branch exists only to read rows written before the
// opaque reference was introduced, and it is the whole of the remaining debt.
// If a generator ever stopped producing the prefix it would quietly acquire a
// new producer, and the branch could never be retired. This test is what stops
// that: the retirement condition becomes purely "no pre-change row survives",
// which is a data question, not a code one.
func TestEveryGeneratedCredentialCarriesTheOpaquePrefix(t *testing.T) {
	secret := []byte("compatibility-prefix-retirement-secret")
	now := time.Now().UTC()

	for _, challenge := range []string{
		"dispatch:pickup:trip_1",
		"dispatch:delivery:trip_1",
		"dispatch:pickup:trip_2",
	} {
		record, err := NewCompatibilityCodeRecord(secret, challenge, now)
		if err != nil {
			t.Fatalf("generate for %s: %v", challenge, err)
		}
		if !strings.HasPrefix(record.Code, "handover_") {
			t.Fatalf("generated credential for %s is stored as %q, which would be read back as legacy plaintext", challenge, record.Code)
		}
		// The stored value must not itself be the displayable code: that is
		// the point of the opaque reference.
		displayed, err := CompatibilityCode(secret, challenge, record.Code)
		if err != nil {
			t.Fatalf("resolve for %s: %v", challenge, err)
		}
		if displayed == record.Code {
			t.Fatalf("the stored reference and the displayed code are the same value for %s", challenge)
		}
	}
}

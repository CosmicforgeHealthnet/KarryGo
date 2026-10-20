package webhooks

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"testing"
)

func TestSignPayloadIsDeterministic(t *testing.T) {
	body := []byte(`{"event":"delivery.completed"}`)
	a := SignPayload("whsec_secret", 1000, body)
	b := SignPayload("whsec_secret", 1000, body)
	if a != b {
		t.Fatalf("signature not deterministic: %q vs %q", a, b)
	}
}

func TestSignPayloadFormatAndMessage(t *testing.T) {
	secret := "whsec_secret"
	body := []byte(`{"event":"delivery.completed","x":1}`)
	var ts int64 = 1717581600

	got := SignPayload(secret, ts, body)
	if !strings.HasPrefix(got, "sha256=") {
		t.Fatalf("signature = %q, want sha256= prefix", got)
	}

	// Recompute HMAC over the exact message: timestamp + "." + raw body.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(ts, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if got != want {
		t.Fatalf("signature = %q, want %q", got, want)
	}
}

func TestSignPayloadChangesWithBody(t *testing.T) {
	s1 := SignPayload("s", 1, []byte(`{"a":1}`))
	s2 := SignPayload("s", 1, []byte(`{"a":2}`))
	if s1 == s2 {
		t.Fatal("signature did not change with body")
	}
}

func TestGenerateSigningSecretFormat(t *testing.T) {
	secret, err := generateSigningSecret()
	if err != nil {
		t.Fatalf("generateSigningSecret() error = %v", err)
	}
	if !strings.HasPrefix(secret, "whsec_") {
		t.Fatalf("secret = %q, want whsec_ prefix", secret)
	}
	if len(secret) != len("whsec_")+32 {
		t.Fatalf("secret len = %d, want %d", len(secret), len("whsec_")+32)
	}
}

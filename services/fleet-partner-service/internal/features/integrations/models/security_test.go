package models

import (
	"strings"
	"testing"
	"time"
)

func TestClientSecretsAreRandomHashedAndComparedExactly(t *testing.T) {
	one, err := RandomToken(32)
	if err != nil {
		t.Fatal(err)
	}
	two, err := RandomToken(32)
	if err != nil {
		t.Fatal(err)
	}
	if one == two || len(one) < 40 {
		t.Fatalf("credentials are not independently random: %q %q", one, two)
	}
	key := []byte("0123456789abcdef0123456789abcdef")
	hashed := HashSecret(key, one)
	if strings.Contains(string(hashed), one) || !SecretMatches(key, one, hashed) || SecretMatches(key, two, hashed) {
		t.Fatal("secret hashing or constant-time comparison contract failed")
	}
	if !ContainsExact([]string{ScopeIntegrationContextRead}, ScopeIntegrationContextRead) ||
		ContainsExact([]string{ScopeIntegrationContextRead + ".admin"}, ScopeIntegrationContextRead) {
		t.Fatal("scope comparison must be exact")
	}
}

func TestWebhookSecretEncryptionRoundTripDoesNotStorePlaintext(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	secret := "webhook-secret-that-must-not-be-stored"
	ciphertext, err := EncryptSecret(key, secret)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(ciphertext), secret) {
		t.Fatal("ciphertext contains plaintext secret")
	}
	plain, err := DecryptSecret(key, ciphertext)
	if err != nil || plain != secret {
		t.Fatalf("round trip failed: %q %v", plain, err)
	}
	ciphertext[0] ^= 0xff
	if _, err = DecryptSecret(key, ciphertext); err == nil {
		t.Fatal("tampered ciphertext was accepted")
	}
}

func TestWebhookSignatureBindsBodyIdentifiersAndReplayWindow(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	body := []byte(`{"eventId":"evt_1","data":{"status":"CONFIRMED"}}`)
	header := SignWebhook("sub_1", "del_1", "evt_1", now.Unix(), body,
		[]SigningSecret{{Version: 2, Secret: "current"}, {Version: 1, Secret: "previous"}})
	for _, tc := range []struct {
		name    string
		secret  string
		version int
		body    []byte
		now     time.Time
		wantErr bool
	}{
		{name: "current", secret: "current", version: 2, body: body, now: now},
		{name: "previous overlap", secret: "previous", version: 1, body: body, now: now},
		{name: "tampered body", secret: "current", version: 2, body: []byte(`{"eventId":"evt_2"}`), now: now, wantErr: true},
		{name: "wrong delivery", secret: "current", version: 2, body: body, now: now, wantErr: true},
		{name: "expired timestamp", secret: "current", version: 2, body: body, now: now.Add(6 * time.Minute), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deliveryID := "del_1"
			if tc.name == "wrong delivery" {
				deliveryID = "del_other"
			}
			err := VerifyWebhookSignature(tc.secret, tc.version, "sub_1", deliveryID, "evt_1", now.Unix(), tc.body, header, tc.now, 5*time.Minute)
			if (err != nil) != tc.wantErr {
				t.Fatalf("VerifyWebhookSignature() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestWebhookPayloadRejectsSecretsRecursively(t *testing.T) {
	if err := ValidateWebhookPayload([]byte(`{"data":{"nested":[{"access_token":"leak"}]}}`)); err == nil {
		t.Fatal("prohibited nested credential field accepted")
	}
	if err := ValidateWebhookPayload([]byte(`{"data":{"status":"CONFIRMED","amountMinor":100}}`)); err != nil {
		t.Fatalf("safe payload rejected: %v", err)
	}
}

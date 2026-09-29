package sharemodels

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestTokenEntropyAndNoSecretsInMetadata(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		token, err := Token()
		raw, e := base64.RawURLEncoding.DecodeString(token)
		if err != nil || e != nil || len(raw) != 32 || seen[token] {
			t.Fatal("token generation failed")
		}
		seen[token] = true
		s := Session{TokenHash: Digest(token), CustomerID: "owner"}
		data, _ := json.Marshal(s)
		if strings.Contains(string(data), token) || strings.Contains(string(data), s.TokenHash) || strings.Contains(string(data), "owner") {
			t.Fatal("secret in metadata")
		}
	}
}
func TestValidationAndExpiry(t *testing.T) {
	for _, in := range []Create{{Authority: "TAXI", SourceRef: "x", ProductCode: "SEND"}, {Authority: "TAXI", SourceRef: "x", ProductCode: "RIDE", Permissions: []string{"payment"}}, {Authority: "TAXI", SourceRef: "x", ProductCode: "RIDE", Permissions: []string{"location", "location"}}} {
		if in.Validate() == nil {
			t.Fatal("invalid source accepted")
		}
	}
	now := time.Now()
	s := Session{Status: "ACTIVE", ExpiresAt: now}
	if s.Readable(now) {
		t.Fatal("expiry boundary accepted")
	}
	s.ExpiresAt = now.Add(time.Hour)
	s.Status = "REVOKED"
	if s.Readable(now) {
		t.Fatal("revoked accepted")
	}
}

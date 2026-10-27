package partneraccount

import (
	"net/mail"
	"strings"
	"testing"
	"time"
)

func TestPartnerOTPEmailMessageHasDateAndMessageID(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	raw := partnerOTPEmailMessageAt("Karryam", "no-reply@karryam.com", "partner@example.com", "123456", now)
	parsed, err := mail.ReadMessage(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatalf("message does not parse: %v", err)
	}
	if got := parsed.Header.Get("From"); got != `"Karryam" <no-reply@karryam.com>` {
		t.Fatalf("From = %q", got)
	}
	date, err := parsed.Header.Date()
	if err != nil || !date.Equal(now) {
		t.Fatalf("Date = %v (%v)", date, err)
	}
	id := parsed.Header.Get("Message-ID")
	if !strings.HasPrefix(id, "<") || !strings.HasSuffix(id, ".partner-otp@karryam.com>") {
		t.Fatalf("Message-ID = %q", id)
	}
	other := partnerOTPEmailMessageAt("Karryam", "no-reply@karryam.com", "partner@example.com", "123456", now)
	otherParsed, _ := mail.ReadMessage(strings.NewReader(string(other)))
	if otherParsed.Header.Get("Message-ID") == id {
		t.Fatal("Message-ID must be unique per message")
	}
}

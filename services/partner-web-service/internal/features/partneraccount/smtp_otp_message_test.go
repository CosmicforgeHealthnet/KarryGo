package partneraccount

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func TestPartnerOTPEmailMessageHasDateAndMessageID(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	raw, err := partnerOTPEmailMessageAt("Karryam", "no-reply@karryam.com", "partner@example.com", "123456", now)
	if err != nil {
		t.Fatal(err)
	}
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
	other, err := partnerOTPEmailMessageAt("Karryam", "no-reply@karryam.com", "partner@example.com", "123456", now)
	if err != nil {
		t.Fatal(err)
	}
	otherParsed, _ := mail.ReadMessage(strings.NewReader(string(other)))
	if otherParsed.Header.Get("Message-ID") == id {
		t.Fatal("Message-ID must be unique per message")
	}
}

func TestPartnerOTPHasOfficialBrandAndCompatibleAlternatives(t *testing.T) {
	raw, err := partnerOTPEmailMessage("KarryGo", "no-reply@karryam.com", "fixture@example.com", "012345")
	if err != nil {
		t.Fatal(err)
	}
	message, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if message.Header.Get("From") != `"Karryam" <no-reply@karryam.com>` || message.Header.Get("Subject") != "Your Karryam verification code" {
		t.Fatal("incorrect partner sender or subject")
	}
	kind, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if err != nil || kind != "multipart/related" {
		t.Fatal("missing related MIME root")
	}
	root := multipart.NewReader(message.Body, params["boundary"])
	alt, err := root.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	kind, params, err = mime.ParseMediaType(alt.Header.Get("Content-Type"))
	if err != nil || kind != "multipart/alternative" {
		t.Fatal("missing alternatives")
	}
	alternatives := multipart.NewReader(alt, params["boundary"])
	for _, kind := range []string{"text/plain", "text/html"} {
		part, err := alternatives.NextPart()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(part)
		if err != nil || !strings.HasPrefix(part.Header.Get("Content-Type"), kind) {
			t.Fatal("incorrect alternative")
		}
		if !bytes.Contains(body, []byte("Your Karryam verification code is 012345. It expires in 10 minutes.")) {
			t.Fatal("partner alternative lost code or expiry")
		}
		if kind == "text/html" && !bytes.Contains(body, []byte("cid:karryam-logo")) {
			t.Fatal("HTML does not reference logo")
		}
	}
	logo, err := root.NextPart()
	if err != nil || logo.Header.Get("Content-ID") != "<karryam-logo>" || logo.Header.Get("Content-Disposition") != `inline; filename="karryam-logo.png"` {
		t.Fatal("missing inline logo")
	}
	data, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, logo))
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(data)) != "35de10e0c2e3f4da2a82cf2a61d663c9a3901c3e0f1b854551fd3f3f412f127b" {
		t.Fatal("partner logo differs from official original")
	}
}

func TestPartnerOTPHTMLAndHeadersAreSafe(t *testing.T) {
	if _, err := partnerOTPEmailMessage("Karryam", "no-reply@karryam.com", "fixture@example.com\r\nBcc: injected@example.com", "012345"); err == nil {
		t.Fatal("recipient injected a header")
	}
	raw, err := partnerOTPEmailMessage("Karryam\r\nBcc: injected@example.com", "no-reply@karryam.com", "fixture@example.com", `<script>unsafe</script>`)
	if err != nil {
		t.Fatal(err)
	}
	message, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil || message.Header.Get("Bcc") != "" {
		t.Fatal("display name injected a header")
	}
	_, params, err := mime.ParseMediaType(message.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	alternative, err := multipart.NewReader(message.Body, params["boundary"]).NextPart()
	if err != nil {
		t.Fatal(err)
	}
	_, params, err = mime.ParseMediaType(alternative.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	parts := multipart.NewReader(alternative, params["boundary"])
	if _, err := parts.NextPart(); err != nil {
		t.Fatal(err)
	}
	html, err := parts.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(html)
	if err != nil || bytes.Contains(body, []byte("<script>")) || !bytes.Contains(body, []byte("&lt;script&gt;")) {
		t.Fatal("unescaped markup in HTML")
	}
}

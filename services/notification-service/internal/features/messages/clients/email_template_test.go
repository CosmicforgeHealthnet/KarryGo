package messageclients

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBrandedEmailPreservesPlainTextAndEmbedsLogo(t *testing.T) {
	message := EmailMessage{
		To: "owner@example.com", Subject: "Your Karryam verification code",
		Body:          "Your verification code is 012345. It expires shortly.",
		SourceService: "vendor-service", EventType: "vendor.auth.login.otp",
	}
	raw, err := formatEmail("Karryam <noreply@example.com>", message)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	kind, params, err := mime.ParseMediaType(parsed.Header.Get("Content-Type"))
	if err != nil || kind != "multipart/related" {
		t.Fatalf("wrong root MIME type: %s %v", kind, err)
	}
	related := multipart.NewReader(parsed.Body, params["boundary"])
	part, err := related.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	kind, params, err = mime.ParseMediaType(part.Header.Get("Content-Type"))
	if err != nil || kind != "multipart/alternative" {
		t.Fatalf("missing text fallback: %s %v", kind, err)
	}
	alternatives := multipart.NewReader(part, params["boundary"])
	plain, err := alternatives.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	text, err := io.ReadAll(plain)
	if err != nil || string(text) != message.Body {
		t.Fatalf("plain text changed: %q %v", text, err)
	}
	htmlPart, err := alternatives.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	html, err := io.ReadAll(htmlPart)
	if err != nil || !bytes.Contains(html, []byte("012345")) || !bytes.Contains(html, []byte("cid:karryam-logo")) {
		t.Fatalf("missing code or inline logo: %v", err)
	}
	logo, err := related.NextPart()
	if err != nil || logo.Header.Get("Content-ID") != "<karryam-logo>" {
		t.Fatalf("missing logo MIME part: %v", err)
	}
	image, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, logo))
	if err != nil || !bytes.Equal(image, emailLogo) {
		t.Fatalf("logo does not match brand asset: %v", err)
	}
}

func TestEmailHeadersAndHTMLCannotInjectMarkup(t *testing.T) {
	message := EmailMessage{To: "owner@example.com", Subject: "Karryam \u2014 update\r\nBcc: other@example.com",
		Body: `<script>alert("unsafe")</script><img src="https://tracker.example">`}
	raw, err := formatEmail("Karryam <noreply@example.com>", message)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Header.Get("Bcc") != "" {
		t.Fatal("subject injected a header")
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(parsed.Header.Get("Subject"))
	if err != nil || subject != cleanEmailHeader(message.Subject) {
		t.Fatalf("Unicode subject corrupted: %q %v", subject, err)
	}
	html, err := renderEmailHTML(message)
	if err != nil || bytes.Contains(html, []byte("<script>")) || bytes.Contains(html, []byte(`<img src="https://tracker.example">`)) {
		t.Fatalf("untrusted email markup rendered: %v", err)
	}
}

func TestVendorAuthenticationSubjectUsesCurrentBrand(t *testing.T) {
	message := EmailMessage{SourceService: "vendor-service", EventType: "vendor.auth.login.otp",
		Subject: "Your KarryGo Vendor verification code", Body: "Your verification code is 123456."}
	if got := emailContent(message).Subject; got != "Your Karryam Vendor verification code" {
		t.Fatalf("legacy brand in subject: %q", got)
	}
}

func TestEmailVariantsAndPreviews(t *testing.T) {
	cases := []struct {
		name          string
		message       EmailMessage
		code, heading string
	}{
		{"vendor", EmailMessage{SourceService: "vendor-service", EventType: "vendor.auth.login.otp",
			Subject: "Your Karryam verification code", Body: "Your KarryGo Vendor verification code is 024816. It expires shortly."}, "024816", "Verify it's you."},
		{"rider", EmailMessage{SourceService: "driver-dispatch-delivery-service", EventType: "dispatch.auth.otp",
			Subject: "Your Karryam verification code", Body: "Your Karryam verification code is 731209. It expires shortly."}, "731209", "Verify it's you."},
		{"confirmation", EmailMessage{SourceService: "vendor-service", EventType: "vendor.auth.step_up.otp",
			Subject: "Confirm this action", Body: "Your confirmation code is 102938. It expires shortly."}, "102938", "Confirm your action."},
		{"notification", EmailMessage{Subject: "Your account update", Body: "Your account details have been updated.\nOpen the app to review your details."}, "", "Your account update"},
		{"unrelated-code", EmailMessage{SourceService: "vendor-service", EventType: "order.updated",
			Subject: "Order update", Body: "Your verification code is 123456."}, "", "Order update"},
		{"missing-code", EmailMessage{SourceService: "vendor-service", EventType: "vendor.auth.login.otp",
			Subject: "Verification update", Body: "Request another code in the app."}, "", "Verification update"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			view := emailContent(item.message)
			if view.Code != item.code || view.Heading != item.heading || view.Verification != (item.code != "") {
				t.Fatalf("incorrect email variant: %+v", view)
			}
			html, err := renderEmailHTML(item.message)
			if err != nil {
				t.Fatal(err)
			}
			if directory := os.Getenv("NOTIFICATION_EMAIL_PREVIEW_DIR"); directory != "" {
				if err := os.MkdirAll(directory, 0o755); err != nil {
					t.Fatal(err)
				}
				preview := strings.ReplaceAll(string(html), "cid:karryam-logo", "data:image/png;base64,"+base64.StdEncoding.EncodeToString(emailLogo))
				if err := os.WriteFile(filepath.Join(directory, item.name+".html"), []byte(preview), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

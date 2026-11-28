package emailbrand

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testAcceptURL = "https://fleet.example.test/sign-up?token=SECRET-TOKEN-VALUE"

func sampleInvitation() Layout {
	return Layout{
		Product: "Karryam Fleet", Eyebrow: "Team invitation",
		Heading:  "Join Cosmicforge on Karryam Fleet",
		Greeting: "Hi Ada,",
		Intro:    []string{"Grace Okafor has invited you to join Cosmicforge on Karryam Fleet."},
		Details: &Card{Title: "Invitation details", Rows: []Row{
			{Label: "Organization", Value: "Cosmicforge"}, {Label: "Role", Value: "Company Administrator"},
			{Label: "Expires", Value: "5 Dec 2026, 10:14 UTC"},
		}},
		Action:       &Action{Label: "Accept invitation", URL: testAcceptURL},
		FallbackLink: true,
		StepsTitle:   "How to activate your account",
		Steps:        []string{"Select Accept invitation.", "Confirm your email and phone number.", "Enter the code we email you."},
		Notice:       &Notice{Tone: ToneSecurity, Title: "Security notice", Text: "Karryam never asks for a password."},
		Support:      true,
	}
}

func parseParts(t *testing.T, raw []byte) (string, map[string]string) {
	t.Helper()
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("parse message: %v", err)
	}
	mediaType, params, err := mime.ParseMediaType(msg.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("content type: %v", err)
	}
	parts := map[string]string{}
	reader := multipart.NewReader(msg.Body, params["boundary"])
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("next part: %v", err)
		}
		kind, _, _ := mime.ParseMediaType(part.Header.Get("Content-Type"))
		body, _ := io.ReadAll(quotedprintable.NewReader(part))
		parts[kind] = string(body)
	}
	return mediaType, parts
}

func TestLayoutRendersComponentsAndHidesTokenFromBody(t *testing.T) {
	layout := sampleInvitation()
	raw, err := Format("no-reply@karryam.com", Message{
		DeliveryID: "d-1", CreatedAt: time.Date(2026, 11, 28, 10, 0, 0, 0, time.UTC),
		To: "ada@example.com", Subject: "You have been invited to Cosmicforge",
		SourceService: "fleet-partner-service", EventType: "corporate.staff.invitation", Layout: &layout,
	})
	if err != nil {
		t.Fatalf("Format() error = %v", err)
	}
	mediaType, parts := parseParts(t, raw)
	if mediaType != "multipart/alternative" {
		t.Fatalf("structured email must be multipart/alternative without an inline logo part, got %s", mediaType)
	}
	html, text := parts["text/html"], parts["text/plain"]
	for _, want := range []string{LogoURL, "Join Cosmicforge on Karryam Fleet", "Hi Ada,", "Company Administrator",
		"Accept invitation", "Open this link in your browser", "Security notice", "mailto:" + SupportEmail,
		"&copy; 2026 Karryam", "max-width:600px", "prefers-color-scheme:dark", ">Fleet<"} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML is missing %q", want)
		}
	}
	// The token may appear only inside href attributes, never as visible text.
	visible := html
	for strings.Contains(visible, `href="`) {
		start := strings.Index(visible, `href="`)
		end := strings.Index(visible[start+6:], `"`)
		visible = visible[:start] + visible[start+6+end+1:]
	}
	if strings.Contains(visible, "SECRET-TOKEN-VALUE") {
		t.Fatal("the invitation token must not be printed in the HTML body")
	}
	if strings.Contains(html, "cid:") {
		t.Fatal("structured email must use the absolute https logo, not an inline cid image")
	}
	for _, want := range []string{"Accept invitation: " + testAcceptURL, "Role: Company Administrator",
		"1. Select Accept invitation.", "support@karryam.com", "automated message"} {
		if !strings.Contains(text, want) {
			t.Errorf("plain text is missing %q", want)
		}
	}
	again, err := Format("no-reply@karryam.com", Message{
		DeliveryID: "d-1", CreatedAt: time.Date(2026, 11, 28, 10, 0, 0, 0, time.UTC),
		To: "ada@example.com", Subject: "You have been invited to Cosmicforge",
		SourceService: "fleet-partner-service", EventType: "corporate.staff.invitation", Layout: &layout,
	})
	if err != nil || !bytes.Equal(raw, again) {
		t.Fatal("a retry of the same delivery must render byte-identical content")
	}
}

func TestLayoutEscapesContentAndRejectsUnsafeLinks(t *testing.T) {
	layout := Layout{Heading: `<script>alert(1)</script>`, Intro: []string{`<b>bold</b>`},
		Action:    &Action{Label: "Open", URL: "javascript:alert(1)"},
		Secondary: &Action{Label: "Plain", URL: "http://insecure.example"}, FallbackLink: true}
	html, err := renderLayoutHTML(Message{Subject: "s", SourceService: "fleet-partner-service", Layout: &layout})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(html), "<script>alert") || strings.Contains(string(html), "<b>bold</b>") {
		t.Fatal("layout text must be HTML-escaped")
	}
	if strings.Contains(string(html), "javascript:") || strings.Contains(string(html), "http://insecure") {
		t.Fatal("only absolute https links may be rendered")
	}
}

func TestOtherProductsKeepTheOriginalDesign(t *testing.T) {
	raw, err := Format("no-reply@karryam.com", Message{
		DeliveryID: "d-2", To: "user@example.com", Subject: "Your code",
		Body: "Your verification code is 123456.", SourceService: "customer-service", EventType: "customer.auth.otp",
	})
	if err != nil {
		t.Fatal(err)
	}
	if mediaType, _ := parseParts(t, raw); mediaType != "multipart/related" {
		t.Fatalf("non-Fleet email must keep the inline-logo design, got %s", mediaType)
	}
}

func TestFleetMessageWithoutLayoutIsStructured(t *testing.T) {
	raw, err := Format("no-reply@karryam.com", Message{
		DeliveryID: "d-3", To: "user@example.com", Subject: "Your Karryam verification code",
		Body: "Your verification code is 482913. It expires in 10 minutes.", SourceService: "fleet-partner-service",
		EventType: "corporate.auth.otp",
	})
	if err != nil {
		t.Fatal(err)
	}
	mediaType, parts := parseParts(t, raw)
	if mediaType != "multipart/alternative" || !strings.Contains(parts["text/html"], ">482913<") ||
		!strings.Contains(parts["text/plain"], "Verification code: 482913") {
		t.Fatal("a Fleet code email without a layout must still show the code in the structured design")
	}
}

func TestLayoutFromData(t *testing.T) {
	layout := sampleInvitation()
	got, err := LayoutFromData(map[string]any{LayoutDataKey: layout})
	if err != nil || got == nil || got.Details.Rows[1].Value != "Company Administrator" || got.Action.URL != testAcceptURL {
		t.Fatalf("LayoutFromData round trip failed: %+v %v", got, err)
	}
	if got, err := LayoutFromData(map[string]any{"other": 1}); got != nil || err != nil {
		t.Fatal("absent layout must be nil without error")
	}
	if _, err := LayoutFromData(map[string]any{LayoutDataKey: map[string]any{"intro": []string{"x"}}}); err == nil {
		t.Fatal("a layout without a heading must be rejected")
	}
}

// TestWriteLayoutPreviews writes sample HTML files for visual review when
// EMAILBRAND_PREVIEW_DIR is set. It is a no-op in CI.
func TestWriteLayoutPreviews(t *testing.T) {
	// Always renders (certification gates refuse skipped tests); set
	// EMAILBRAND_PREVIEW_DIR to keep the files for a visual check.
	dir := os.Getenv("EMAILBRAND_PREVIEW_DIR")
	if dir == "" {
		dir = t.TempDir()
	}
	invitation := sampleInvitation()
	otp := Layout{Product: "Karryam Fleet", Eyebrow: "Sign in", Heading: "Your verification code",
		Intro: []string{"Use this code to sign in to Karryam Fleet."},
		Code:  &Code{Label: "Verification code", Value: "482913", Expiry: "Expires in 10 minutes"},
		Notice: &Notice{Tone: ToneSecurity, Title: "Keep this code private",
			Text: "Karryam will never ask for this code. If you did not request it, you can ignore this email."}, Support: true}
	quote := Layout{Product: "Karryam Fleet", Eyebrow: "Quotation", Heading: "Your quotation is ready",
		Status: &Status{Tone: ToneInfo, Label: "Awaiting your response"}, Greeting: "Hello Ada,",
		Intro: []string{"Cosmicforge has issued quotation QUO-2026-0042."},
		Details: &Card{Title: "Summary", Rows: []Row{{Label: "Quotation", Value: "QUO-2026-0042"}, {Label: "Valid until", Value: "12 Dec 2026"}},
			Total: &Row{Label: "Total", Value: "NGN 1,250,000.00"}}, Support: true}
	for name, layout := range map[string]Layout{"invitation": invitation, "otp": otp, "quotation": quote} {
		layout := layout
		html, err := renderLayoutHTML(Message{Subject: layout.Heading, SourceService: "fleet-partner-service",
			CreatedAt: time.Date(2026, 11, 28, 0, 0, 0, 0, time.UTC), Layout: &layout})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name+".html"), html, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

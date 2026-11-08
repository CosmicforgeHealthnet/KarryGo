package emailbrand

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"os"
	"strings"
	"testing"
)

func TestOfficialLogoRemainsByteIdentical(t *testing.T) {
	original, err := os.ReadFile("../../../web/karryam-landing/public/images/logo.png")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, emailLogo) {
		t.Fatal("embedded logo differs from original brand asset")
	}
}

func TestBrandingAcrossEmailFamilies(t *testing.T) {
	for _, event := range []string{
		"customer.auth.otp", "vendor.auth.login.otp", "vendor.auth.step_up.otp",
		"dispatch.auth.otp", "provider.auth.otp", "corporate.auth.otp", "partner.auth.otp",
		"corporate.staff.invitation", "corporate.provider.invitation", "corporate.organization.onboarding_update",
		"booking.completed", "package.completed", "ride.completed", "payment.success",
		"refund.completed", "share.created", "network.invitation", "campaign.email", "dispatch.email",
	} {
		t.Run(event, func(t *testing.T) {
			body := "Your KarryGo verification code is 012345. It expires in 10 minutes. Never share it. Thank you for using Cosmicforge Logistics."
			raw, err := Format("Legacy Name <no-reply@karryam.com>", Message{
				To: "fixture@example.com", EventType: event,
				Subject: "KarryGo — account update", Body: body,
			})
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := mail.ReadMessage(bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			sender, err := mail.ParseAddress(parsed.Header.Get("From"))
			if err != nil || sender.Name != "Karryam" || sender.Address != "no-reply@karryam.com" {
				t.Fatal("incorrect sender identity")
			}
			subject, err := new(mime.WordDecoder).DecodeHeader(parsed.Header.Get("Subject"))
			if err != nil || subject != "Karryam — account update" {
				t.Fatal("incorrect subject")
			}
			_, params, err := mime.ParseMediaType(parsed.Header.Get("Content-Type"))
			if err != nil {
				t.Fatal(err)
			}
			related := multipart.NewReader(parsed.Body, params["boundary"])
			alternative, err := related.NextPart()
			if err != nil {
				t.Fatal(err)
			}
			_, params, err = mime.ParseMediaType(alternative.Header.Get("Content-Type"))
			if err != nil {
				t.Fatal(err)
			}
			parts := multipart.NewReader(alternative, params["boundary"])
			for _, kind := range []string{"text/plain", "text/html"} {
				part, err := parts.NextPart()
				if err != nil {
					t.Fatal(err)
				}
				content, err := io.ReadAll(part)
				if err != nil || !strings.HasPrefix(part.Header.Get("Content-Type"), kind) {
					t.Fatal("missing compatible alternative")
				}
				for _, required := range []string{"Karryam", "012345", "10 minutes", "Never share it"} {
					if !bytes.Contains(content, []byte(required)) {
						t.Fatalf("%s lost required copy", kind)
					}
				}
				if bytes.Contains(content, []byte("KarryGo")) || bytes.Contains(content, []byte("Cosmicforge Logistics")) {
					t.Fatal("legacy email copy remains")
				}
			}
		})
	}
}

func TestAddressHeadersFailClosed(t *testing.T) {
	for _, from := range []string{"", "resend", "Karryam <no-reply@karryam.com>\r\nBcc: attacker@example.com"} {
		if _, err := Format(from, Message{To: "fixture@example.com"}); err == nil {
			t.Fatal("unsafe sender accepted")
		}
	}
	for _, to := range []string{"", "invalid", "fixture@example.com\r\nBcc: attacker@example.com"} {
		if _, err := Format("no-reply@karryam.com", Message{To: to}); err == nil {
			t.Fatal("unsafe recipient accepted")
		}
	}
}

func TestAllAuthenticationCodesAndSecurityCopyRemainVisible(t *testing.T) {
	for source, event := range map[string]string{
		"customer-service": "customer.auth.otp", "vendor-service": "vendor.auth.step_up.otp",
		"driver-dispatch-delivery-service": "dispatch.auth.otp", "driver-taxi-service": "provider.auth.otp",
		"driver-hauling-service": "provider.auth.otp", "fleet-partner-service": "corporate.auth.otp",
		"partner-web-service": "partner.auth.otp",
	} {
		t.Run(source, func(t *testing.T) {
			body := "Your verification code is: 012345. It expires in 7 minutes. Only confirms the action you requested — never share it."
			view := emailContent(Message{SourceService: source, EventType: event, Body: body})
			if !view.Verification || view.Code != "012345" || view.Body != body {
				t.Fatal("OTP or security copy changed")
			}
			html, err := renderEmailHTML(Message{SourceService: source, EventType: event, Body: body})
			if err != nil || !bytes.Contains(html, []byte(body)) {
				t.Fatal("HTML hides original security or expiry copy")
			}
		})
	}
}

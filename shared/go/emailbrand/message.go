// Package emailbrand renders the existing Karryam email design for SMTP adapters.
package emailbrand

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"regexp"
	"strings"
	"time"
)

//go:embed email_assets/karryam-logo.png
var emailLogo []byte

//go:embed email_assets/message.html
var emailTemplateSource string

var emailTemplate = template.Must(template.New("karryam-email").Parse(emailTemplateSource))
var verificationCode = regexp.MustCompile(`(?i)\b(?:verification|confirmation|sign-in) code is[:\s]+([0-9]{4,8})\b`)

type emailView struct {
	Subject, Heading, Intro, Body, Code, Account, Preheader string
	Verification                                            bool
}

func emailContent(message Message) emailView {
	subject := emailSubject(message)
	view := emailView{
		Subject: subject, Heading: subject,
		Body: brandCopy(message.Body), Account: "KARRYAM", Preheader: subject,
	}
	if message.SourceService == "vendor-service" {
		view.Account = "VENDOR ACCOUNT"
	} else if message.SourceService == "driver-dispatch-delivery-service" {
		view.Account = "RIDER ACCOUNT"
	}
	isVendorOTP := message.SourceService == "vendor-service" &&
		strings.HasPrefix(message.EventType, "vendor.auth.") && strings.HasSuffix(message.EventType, ".otp")
	isDispatchOTP := message.SourceService == "driver-dispatch-delivery-service" && message.EventType == "dispatch.auth.otp"
	isCorporateOTP := message.SourceService == "fleet-partner-service" && message.EventType == "corporate.auth.otp"
	isOtherOTP := map[string]string{
		"customer-service":       "customer.auth.otp",
		"driver-taxi-service":    "provider.auth.otp",
		"driver-hauling-service": "provider.auth.otp",
		"partner-web-service":    "partner.auth.otp",
	}[message.SourceService] == message.EventType && message.EventType != ""
	if !isVendorOTP && !isDispatchOTP && !isCorporateOTP && !isOtherOTP {
		return view
	}
	match := verificationCode.FindStringSubmatch(message.Body)
	if len(match) != 2 {
		return view
	}
	view.Verification, view.Code = true, match[1]
	view.Heading = "Verify it's you."
	view.Intro = "Enter this code to continue securely."
	view.Preheader = "Your Karryam verification code is ready. Never share it with anyone."
	if message.EventType == "vendor.auth.step_up.otp" {
		view.Heading = "Confirm your action."
		view.Intro = "Enter this code in the app to confirm the action you requested."
	}
	return view
}

func renderEmailHTML(message Message) ([]byte, error) {
	var html bytes.Buffer
	if err := emailTemplate.Execute(&html, emailContent(message)); err != nil {
		return nil, fmt.Errorf("render Karryam email: %w", err)
	}
	return html.Bytes(), nil
}

func cleanEmailHeader(value string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(value)
}

// Only presentation copy is normalized. Stored notifications, event contracts and
// delivery identities remain unchanged, including on retries.
func brandCopy(value string) string {
	return strings.NewReplacer("Cosmicforge Logistics", "Karryam", "KarryGo", "Karryam", "Karry Go", "Karryam").Replace(value)
}

func emailSubject(message Message) string {
	return brandCopy(cleanEmailHeader(message.Subject))
}

// Sender preserves the configured authorized address and uses the official name.
// A missing or invalid address fails closed; it is never replaced with a guess.
func Sender(from string) (*mail.Address, error) {
	if strings.ContainsAny(from, "\r\n") {
		return nil, fmt.Errorf("smtp sender address is invalid")
	}
	address, err := mail.ParseAddress(from)
	if err != nil {
		return nil, fmt.Errorf("smtp sender address is invalid")
	}
	address.Name = "Karryam"
	return address, nil
}

// Message holds presentation and the existing persisted delivery identity.
type Message struct {
	DeliveryID    string
	MessageID     string
	CreatedAt     time.Time
	To            string
	Subject       string
	Body          string
	SourceService string
	EventType     string
}

// Format keeps the existing inline logo, text/HTML alternatives and retry payload.
func Format(from string, message Message) ([]byte, error) {
	sender, err := Sender(from)
	if err != nil {
		return nil, err
	}
	from = sender.String()
	if strings.ContainsAny(message.To, "\r\n") {
		return nil, fmt.Errorf("recipient email is invalid")
	}
	recipient, err := mail.ParseAddress(message.To)
	if err != nil {
		return nil, fmt.Errorf("recipient email is invalid")
	}
	message.To = recipient.String()
	html, err := renderEmailHTML(message)
	if err != nil {
		return nil, err
	}
	var relatedBody bytes.Buffer
	related := multipart.NewWriter(&relatedBody)
	var alternatives bytes.Buffer
	alternative := multipart.NewWriter(&alternatives)
	// A delivery keeps one provider idempotency key through queue retries.
	// Keep MIME boundaries and Date stable too: an accepted send with a lost
	// reply must retry the same payload, rather than conflict with that key.
	var deliveryKey string
	if message.DeliveryID != "" {
		digest := sha256.Sum256([]byte("karrygo-email/" + message.DeliveryID))
		deliveryKey = fmt.Sprintf("%x", digest)
		if err := related.SetBoundary("related-" + deliveryKey[:48]); err != nil {
			return nil, err
		}
		if err := alternative.SetBoundary("alternative-" + deliveryKey[:48]); err != nil {
			return nil, err
		}
	}
	for _, content := range []struct{ kind, body string }{
		{"text/plain", brandCopy(message.Body)}, {"text/html", string(html)},
	} {
		part, err := alternative.CreatePart(textproto.MIMEHeader{
			"Content-Type":              {content.kind + "; charset=UTF-8"},
			"Content-Transfer-Encoding": {"quoted-printable"},
		})
		if err != nil {
			return nil, err
		}
		writer := quotedprintable.NewWriter(part)
		if _, err := writer.Write([]byte(content.body)); err != nil {
			return nil, err
		}
		if err := writer.Close(); err != nil {
			return nil, err
		}
	}
	if err := alternative.Close(); err != nil {
		return nil, err
	}
	part, err := related.CreatePart(textproto.MIMEHeader{
		"Content-Type": {mime.FormatMediaType("multipart/alternative", map[string]string{"boundary": alternative.Boundary()})},
	})
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(alternatives.Bytes()); err != nil {
		return nil, err
	}
	image, err := related.CreatePart(textproto.MIMEHeader{
		"Content-Type":              {"image/png"},
		"Content-ID":                {"<karryam-logo>"},
		"Content-Disposition":       {`inline; filename="karryam-logo.png"`},
		"Content-Transfer-Encoding": {"base64"},
	})
	if err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(emailLogo)
	for len(encoded) > 0 {
		length := min(76, len(encoded))
		if _, err := fmt.Fprint(image, encoded[:length]+"\r\n"); err != nil {
			return nil, err
		}
		encoded = encoded[length:]
	}
	if err := related.Close(); err != nil {
		return nil, err
	}
	createdAt := message.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	headers := []string{
		"From: " + cleanEmailHeader(from), "To: " + cleanEmailHeader(message.To),
		"Subject: " + mime.QEncoding.Encode("UTF-8", emailSubject(message)),
		"Date: " + createdAt.UTC().Format(time.RFC1123Z),
		"MIME-Version: 1.0",
		"Content-Type: " + mime.FormatMediaType("multipart/related", map[string]string{
			"boundary": related.Boundary(), "type": "multipart/alternative",
		}),
	}
	if deliveryKey != "" {
		headers = append(headers, "Resend-Idempotency-Key: karrygo/"+deliveryKey)
		if sender, err := mail.ParseAddress(from); err == nil {
			if at := strings.LastIndex(sender.Address, "@"); at >= 0 {
				headers = append(headers, "Message-ID: <"+deliveryKey+"@"+sender.Address[at+1:]+">")
			}
		}
	} else if message.MessageID != "" {
		headers = append(headers, "Message-ID: "+cleanEmailHeader(message.MessageID))
	}
	return append([]byte(strings.Join(headers, "\r\n")+"\r\n\r\n"), relatedBody.Bytes()...), nil
}

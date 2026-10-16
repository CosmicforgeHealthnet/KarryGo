package messageclients

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"fmt"
	"html/template"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
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
var verificationCode = regexp.MustCompile(`(?i)\b(?:verification|confirmation) code is ([0-9]{4,8})\b`)

type emailView struct {
	Subject, Heading, Intro, Body, Code, Account, Preheader string
	Verification                                            bool
}

func emailContent(message EmailMessage) emailView {
	subject := emailSubject(message)
	view := emailView{
		Subject: subject, Heading: subject,
		Body: message.Body, Account: "KARRYAM", Preheader: subject,
	}
	if message.SourceService == "vendor-service" {
		view.Account = "VENDOR ACCOUNT"
	} else if message.SourceService == "driver-dispatch-delivery-service" {
		view.Account = "RIDER ACCOUNT"
	}
	isVendorOTP := message.SourceService == "vendor-service" &&
		strings.HasPrefix(message.EventType, "vendor.auth.") && strings.HasSuffix(message.EventType, ".otp")
	isDispatchOTP := message.SourceService == "driver-dispatch-delivery-service" && message.EventType == "dispatch.auth.otp"
	if !isVendorOTP && !isDispatchOTP {
		return view
	}
	match := verificationCode.FindStringSubmatch(message.Body)
	if len(match) != 2 {
		return view
	}
	view.Verification, view.Code = true, match[1]
	view.Heading = "Verify it's you."
	view.Intro = "Enter this code in the app to continue securely."
	view.Preheader = "Your Karryam verification code is ready. Never share it with anyone."
	if message.EventType == "vendor.auth.step_up.otp" {
		view.Heading = "Confirm your action."
		view.Intro = "Enter this code in the app to confirm the action you requested."
	}
	return view
}

func renderEmailHTML(message EmailMessage) ([]byte, error) {
	var html bytes.Buffer
	if err := emailTemplate.Execute(&html, emailContent(message)); err != nil {
		return nil, fmt.Errorf("render Karryam email: %w", err)
	}
	return html.Bytes(), nil
}

func cleanEmailHeader(value string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(value)
}

func emailSubject(message EmailMessage) string {
	subject := cleanEmailHeader(message.Subject)
	if message.SourceService == "vendor-service" && strings.HasPrefix(message.EventType, "vendor.auth.") {
		subject = strings.ReplaceAll(subject, "KarryGo Vendor", "Karryam Vendor")
	}
	return subject
}

func formatEmail(from string, message EmailMessage) ([]byte, error) {
	html, err := renderEmailHTML(message)
	if err != nil {
		return nil, err
	}
	var relatedBody bytes.Buffer
	related := multipart.NewWriter(&relatedBody)
	var alternatives bytes.Buffer
	alternative := multipart.NewWriter(&alternatives)
	for _, content := range []struct{ kind, body string }{
		{"text/plain", message.Body}, {"text/html", string(html)},
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
	headers := []string{
		"From: " + cleanEmailHeader(from), "To: " + cleanEmailHeader(message.To),
		"Subject: " + mime.QEncoding.Encode("UTF-8", emailSubject(message)),
		"Date: " + time.Now().Format(time.RFC1123Z),
		"MIME-Version: 1.0",
		"Content-Type: " + mime.FormatMediaType("multipart/related", map[string]string{
			"boundary": related.Boundary(), "type": "multipart/alternative",
		}),
	}
	return append([]byte(strings.Join(headers, "\r\n")+"\r\n\r\n"), relatedBody.Bytes()...), nil
}

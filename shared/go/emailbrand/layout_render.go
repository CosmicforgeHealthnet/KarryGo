package emailbrand

import (
	"bytes"
	_ "embed"
	"fmt"
	"html/template"
	"strings"
)

//go:embed email_assets/layout.html
var layoutTemplateSource string

var layoutTemplate = template.Must(template.New("karryam-layout").Funcs(template.FuncMap{
	"inc": func(i int) int { return i + 1 },
}).Parse(layoutTemplateSource))

type layoutButton struct {
	Label   string
	URL     template.URL
	Primary bool
}

type layoutTone struct {
	Bg, Border, Text, Label, Title, Body string
}

type layoutView struct {
	Subject, Preheader, Eyebrow, Heading, Greeting string
	Product, ProductLabel                          string
	LogoURL, WebsiteURL, PrivacyURL, SupportEmail  string
	Intro, Outro, Steps                            []string
	StepsTitle                                     string
	Status, Notice                                 *layoutTone
	Code                                           *Code
	Details                                        *Card
	Buttons                                        []layoutButton
	Fallback                                       template.URL
	Support                                        bool
	Year                                           int
}

// fleetSource is the producer whose emails use the structured layout even
// when they carry no explicit layout (for example a message queued before the
// layout existed): its subject and body are mapped onto the same components.
const fleetSource = "fleet-partner-service"

// usesLayout reports whether a message is rendered with the structured layout.
// Every other product keeps its existing design byte for byte.
func usesLayout(message Message) bool {
	return message.Layout != nil || message.SourceService == fleetSource
}

// effectiveLayout returns the producer's layout, or one derived from the
// subject and plain-text body for a Fleet message that has none.
func effectiveLayout(message Message) Layout {
	if message.Layout != nil {
		return *message.Layout
	}
	layout := Layout{Product: "Karryam Fleet", Heading: emailSubject(message), Support: true}
	body := brandCopy(message.Body)
	if match := verificationCode.FindStringSubmatch(body); len(match) == 2 {
		layout.Eyebrow = "Verification"
		layout.Heading = "Your verification code"
		layout.Intro = []string{"Use this code to continue. It can be used once."}
		layout.Code = &Code{Label: "Verification code", Value: match[1]}
		layout.Notice = &Notice{Tone: ToneSecurity, Title: "Keep this code private",
			Text: "Karryam will never ask for this code by phone, chat or email. If you did not request it, you can ignore this email."}
		return layout
	}
	for _, paragraph := range strings.Split(body, "\n\n") {
		if paragraph = strings.TrimSpace(paragraph); paragraph != "" {
			layout.Intro = append(layout.Intro, paragraph)
		}
	}
	return layout
}

func tone(t, label, title, body string) *layoutTone {
	bg, border, text := tonePalette(t)
	return &layoutTone{Bg: bg, Border: border, Text: text, Label: label, Title: title, Body: body}
}

func buildLayoutView(message Message) layoutView {
	layout := effectiveLayout(message)
	product := productName(layout)
	view := layoutView{
		Subject: emailSubject(message), Eyebrow: layout.Eyebrow,
		Heading: brandCopy(layout.Heading), Greeting: layout.Greeting,
		Product: product, LogoURL: LogoURL, WebsiteURL: WebsiteURL,
		PrivacyURL: PrivacyPolicyURL, SupportEmail: SupportEmail,
		Intro: nonEmpty(layout.Intro), Outro: nonEmpty(layout.Outro),
		Steps: nonEmpty(layout.Steps), StepsTitle: layout.StepsTitle,
		Support: layout.Support, Year: copyrightYear(message.CreatedAt),
	}
	if strings.HasPrefix(product, "Karryam ") {
		view.ProductLabel = strings.TrimPrefix(product, "Karryam ")
	}
	view.Preheader = layout.Preheader
	if view.Preheader == "" {
		view.Preheader = view.Heading
	}
	if s := layout.Status; s != nil && strings.TrimSpace(s.Label) != "" {
		view.Status = tone(s.Tone, s.Label, "", "")
	}
	if n := layout.Notice; n != nil && strings.TrimSpace(n.Text) != "" {
		view.Notice = tone(n.Tone, "", n.Title, n.Text)
	}
	if c := layout.Code; c != nil && strings.TrimSpace(c.Value) != "" {
		code := *c
		if code.Label == "" {
			code.Label = "Your code"
		}
		view.Code = &code
	}
	if d := layout.Details; d != nil && (len(d.Rows) > 0 || d.Total != nil) {
		view.Details = d
	}
	for i, action := range []*Action{layout.Action, layout.Secondary} {
		if action == nil || strings.TrimSpace(action.Label) == "" || !SafeURL(action.URL) {
			continue
		}
		// SafeURL admits only absolute https links, so marking the value as a
		// trusted URL cannot smuggle in a javascript: or data: scheme.
		view.Buttons = append(view.Buttons, layoutButton{Label: action.Label, URL: template.URL(strings.TrimSpace(action.URL)), Primary: i == 0})
	}
	if layout.FallbackLink && layout.Action != nil && SafeURL(layout.Action.URL) {
		view.Fallback = template.URL(strings.TrimSpace(layout.Action.URL))
	}
	return view
}

func nonEmpty(values []string) []string {
	var out []string
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func renderLayoutHTML(message Message) ([]byte, error) {
	var html bytes.Buffer
	if err := layoutTemplate.Execute(&html, buildLayoutView(message)); err != nil {
		return nil, fmt.Errorf("render Karryam layout email: %w", err)
	}
	return bytes.TrimSpace(html.Bytes()), nil
}

// layoutText is the plain-text alternative for a structured email.
func layoutText(message Message) string {
	return layoutPlainText(effectiveLayout(message), copyrightYear(message.CreatedAt))
}

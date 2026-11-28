package emailbrand

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// LayoutDataKey is the notification data key that carries a structured email
// layout. A producer puts a Layout there; notification-service passes it to
// Format unchanged, so a retry renders exactly the same email.
const LayoutDataKey = "email_layout"

// Brand constants used by the structured layout. Every link here is a live,
// approved destination; anything that does not yet exist (a privacy policy
// page) is left empty and therefore not rendered.
const (
	LogoURL          = "https://karryam.com/images/logo.png"
	WebsiteURL       = "https://karryam.com"
	SupportEmail     = "support@karryam.com"
	PrivacyPolicyURL = ""
)

// Tone names a colour family. Unknown tones fall back to ToneInfo.
const (
	ToneInfo     = "info"
	ToneSuccess  = "success"
	ToneWarning  = "warning"
	ToneDanger   = "danger"
	ToneSecurity = "security"
	ToneNeutral  = "neutral"
)

// Layout is a structured transactional email. Each field is optional and is
// rendered by one shared component; producers choose the components their
// message needs instead of sending one long paragraph.
type Layout struct {
	// Product is the name shown in the header and footer, e.g. "Karryam Fleet".
	Product   string `json:"product,omitempty"`
	Preheader string `json:"preheader,omitempty"`
	Eyebrow   string `json:"eyebrow,omitempty"`
	Heading   string `json:"heading"`
	Greeting  string `json:"greeting,omitempty"`
	// Intro paragraphs, rendered in order above every component.
	Intro   []string `json:"intro,omitempty"`
	Status  *Status  `json:"status,omitempty"`
	Code    *Code    `json:"code,omitempty"`
	Details *Card    `json:"details,omitempty"`
	Action  *Action  `json:"action,omitempty"`
	// Secondary is rendered as an outline button next to (below) Action.
	Secondary *Action `json:"secondary,omitempty"`
	// FallbackLink renders a discreet "open in browser" link to Action.URL
	// below the button. The URL itself is never printed in the HTML body.
	FallbackLink bool     `json:"fallbackLink,omitempty"`
	StepsTitle   string   `json:"stepsTitle,omitempty"`
	Steps        []string `json:"steps,omitempty"`
	Notice       *Notice  `json:"notice,omitempty"`
	// Outro paragraphs, rendered after the components.
	Outro []string `json:"outro,omitempty"`
	// Support renders the "Need help?" section with the support address.
	Support bool `json:"support,omitempty"`
}

// Status is a small coloured label, e.g. "Approved" or "Action required".
type Status struct {
	Tone  string `json:"tone,omitempty"`
	Label string `json:"label"`
}

// Code is a one-time code display.
type Code struct {
	Label  string `json:"label,omitempty"`
	Value  string `json:"value"`
	Expiry string `json:"expiry,omitempty"`
}

// Card is an information card or transaction summary.
type Card struct {
	Title string `json:"title,omitempty"`
	Rows  []Row  `json:"rows"`
	// Total is rendered last, separated and emphasised (transaction summaries).
	Total *Row `json:"total,omitempty"`
}

// Row is one label/value line of a Card.
type Row struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// Action is a button. Only absolute https URLs are rendered.
type Action struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// Notice is a highlighted warning, security or information box.
type Notice struct {
	Tone  string `json:"tone,omitempty"`
	Title string `json:"title,omitempty"`
	Text  string `json:"text"`
}

// LayoutFromData returns the layout carried in notification data, or nil when
// there is none. A malformed layout is an error rather than a silently
// degraded email.
func LayoutFromData(data map[string]any) (*Layout, error) {
	raw, ok := data[LayoutDataKey]
	if !ok || raw == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, fmt.Errorf("email layout is not encodable: %w", err)
	}
	var layout Layout
	if err := json.Unmarshal(encoded, &layout); err != nil {
		return nil, fmt.Errorf("email layout is malformed: %w", err)
	}
	if strings.TrimSpace(layout.Heading) == "" {
		return nil, fmt.Errorf("email layout has no heading")
	}
	return &layout, nil
}

// SafeURL reports whether a link may be rendered: absolute https only.
func SafeURL(raw string) bool {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	return err == nil && parsed.Scheme == "https" && parsed.Host != "" && parsed.User == nil
}

func normalizeTone(tone string) string {
	switch tone {
	case ToneSuccess, ToneWarning, ToneDanger, ToneSecurity, ToneNeutral:
		return tone
	default:
		return ToneInfo
	}
}

// tonePalette returns (background, border, text) for light mode.
func tonePalette(tone string) (string, string, string) {
	switch normalizeTone(tone) {
	case ToneSuccess:
		return "#ecf8f1", "#1f8a4c", "#14683a"
	case ToneWarning:
		return "#fff7e6", "#b7791f", "#8a5a12"
	case ToneDanger:
		return "#fdeeee", "#c53030", "#9b2424"
	case ToneSecurity:
		return "#eef4fb", "#2b5c9e", "#1f4778"
	case ToneNeutral:
		return "#f2f4f7", "#8592a3", "#46546a"
	default:
		return "#eaf7f5", "#0e7c72", "#0b5f57"
	}
}

// layoutPlainText is the text/plain alternative. It carries everything the
// HTML does, including full URLs, because a plain-text reader has no buttons.
func layoutPlainText(layout Layout, year int) string {
	var b strings.Builder
	line := func(s string) {
		if s = strings.TrimSpace(s); s != "" {
			b.WriteString(s)
			b.WriteString("\n")
		}
	}
	gap := func() { b.WriteString("\n") }
	line(layout.Heading)
	gap()
	if layout.Greeting != "" {
		line(layout.Greeting)
		gap()
	}
	for _, p := range layout.Intro {
		line(p)
		gap()
	}
	if layout.Status != nil && layout.Status.Label != "" {
		line("Status: " + layout.Status.Label)
		gap()
	}
	if layout.Code != nil && layout.Code.Value != "" {
		label := layout.Code.Label
		if label == "" {
			label = "Your code"
		}
		line(label + ": " + layout.Code.Value)
		line(layout.Code.Expiry)
		gap()
	}
	if card := layout.Details; card != nil && (len(card.Rows) > 0 || card.Total != nil) {
		line(card.Title)
		for _, row := range card.Rows {
			line(row.Label + ": " + row.Value)
		}
		if card.Total != nil {
			line(card.Total.Label + ": " + card.Total.Value)
		}
		gap()
	}
	for _, action := range []*Action{layout.Action, layout.Secondary} {
		if action != nil && SafeURL(action.URL) {
			line(action.Label + ": " + action.URL)
		}
	}
	if (layout.Action != nil && SafeURL(layout.Action.URL)) || (layout.Secondary != nil && SafeURL(layout.Secondary.URL)) {
		gap()
	}
	if len(layout.Steps) > 0 {
		line(layout.StepsTitle)
		for i, step := range layout.Steps {
			line(fmt.Sprintf("%d. %s", i+1, step))
		}
		gap()
	}
	if layout.Notice != nil && layout.Notice.Text != "" {
		line(layout.Notice.Title)
		line(layout.Notice.Text)
		gap()
	}
	for _, p := range layout.Outro {
		line(p)
		gap()
	}
	if layout.Support {
		line("Need help? Contact " + SupportEmail + ".")
		gap()
	}
	line("--")
	line(productName(layout))
	line(WebsiteURL)
	if PrivacyPolicyURL != "" {
		line("Privacy Policy: " + PrivacyPolicyURL)
	}
	line("Help and support: " + SupportEmail)
	line(fmt.Sprintf("© %d Karryam. All rights reserved.", year))
	line("This is an automated message. Please do not reply to this email.")
	return b.String()
}

func productName(layout Layout) string {
	if strings.TrimSpace(layout.Product) != "" {
		return strings.TrimSpace(layout.Product)
	}
	return "Karryam"
}

func copyrightYear(createdAt time.Time) int {
	if createdAt.IsZero() {
		return time.Now().UTC().Year()
	}
	return createdAt.UTC().Year()
}

package clients

import (
	"strings"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/emailbrand"
)

func TestStaffInvitationLayoutIsStructured(t *testing.T) {
	const accept = "https://fleet.example.test/sign-up?token=opaque"
	layout := invitationLayout(InvitationMessage{
		Email: "ada@example.com", OrganizationName: "Cosmicforge", InviterName: "Grace Okafor",
		RecipientName: "Ada Lovelace", RoleNames: []string{"Company Administrator"}, AcceptURL: accept,
		ExpiresAt: time.Date(2026, 12, 5, 10, 14, 0, 0, time.UTC), EventType: EventCorporateStaffInvitation,
	})
	if layout.Heading != "Join Cosmicforge on Karryam Fleet" || layout.Greeting != "Hi Ada," {
		t.Fatalf("heading/greeting: %q %q", layout.Heading, layout.Greeting)
	}
	want := map[string]string{"Organization": "Cosmicforge", "Role": "Company Administrator",
		"Invited by": "Grace Okafor", "Expires": "5 Dec 2026, 10:14 UTC"}
	for _, row := range layout.Details.Rows {
		if want[row.Label] == row.Value {
			delete(want, row.Label)
		}
	}
	if len(want) != 0 {
		t.Fatalf("missing invitation details: %v", want)
	}
	if layout.Action == nil || layout.Action.URL != accept || layout.Action.Label != "Accept invitation" || !layout.FallbackLink {
		t.Fatal("invitation must carry the Accept invitation button with a fallback link")
	}
	for _, text := range append(append([]string{}, layout.Intro...), layout.Steps...) {
		if strings.Contains(text, "token=") {
			t.Fatal("the accept URL must never be printed in the email copy")
		}
	}
	if layout.Notice == nil || !layout.Support {
		t.Fatal("invitation must include the security notice and support section")
	}
}

func TestInvitationWithoutAcceptURLHasNoButton(t *testing.T) {
	layout := invitationLayout(InvitationMessage{OrganizationName: "Cosmicforge", EventType: EventCorporateStaffInvitation,
		AcceptURL: "http://insecure.example/sign-up"})
	if layout.Action != nil || layout.FallbackLink {
		t.Fatal("a missing or non-https accept URL must not render a button")
	}
}

func TestProviderNoticeLayoutHasNoAction(t *testing.T) {
	layout := invitationLayout(InvitationMessage{OrganizationName: "Cosmicforge", Email: "p@example.com",
		EventType: EventCorporateProviderLoginNotice})
	if layout.Action != nil || len(layout.Steps) != 3 || layout.Heading == "" {
		t.Fatalf("provider set-up notice must give steps and no link: %+v", layout)
	}
}

func TestOTPLayoutShowsCodeAndPurpose(t *testing.T) {
	layout := otpLayout(OTPMessage{Code: "482913", Purpose: "SIGN_IN", ExpiresIn: 10 * time.Minute})
	if layout.Code == nil || layout.Code.Value != "482913" || !strings.Contains(layout.Code.Expiry, "10 minutes") ||
		layout.Eyebrow != "Sign in" || layout.Notice.Tone != emailbrand.ToneSecurity {
		t.Fatalf("sign-in code layout: %+v", layout)
	}
	stepUp := otpLayout(OTPMessage{Code: "111222", Purpose: "STEP_UP", ExpiresIn: 5 * time.Minute})
	if stepUp.Notice == nil || stepUp.Notice.Tone != emailbrand.ToneWarning {
		t.Fatal("a step-up code must warn the user who did not start the action")
	}
}

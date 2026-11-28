package clients

import (
	"fmt"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/emailbrand"
)

// Structured email layouts for corporate authentication emails. The plain
// title and body sent alongside remain the fallback for any channel or
// renderer that cannot use a layout.

const fleetProduct = "Karryam Fleet"

func otpPurposeCopy(purpose string) (eyebrow, intro string) {
	switch purpose {
	case "SIGN_UP":
		return "Create your account", "Use this code to finish creating your Karryam Fleet account."
	case "SIGN_IN", "SIGN_UP_OR_LOGIN":
		return "Sign in", "Use this code to sign in to Karryam Fleet."
	case "STAFF_INVITATION":
		return "Accept invitation", "Use this code to accept your organization invitation and activate your account."
	case "PROVIDER_INVITATION":
		return "Accept invitation", "Use this code to accept your provider invitation."
	case "ORGANIZATION_SELECTION":
		return "Sign in", "Use this code to continue to your organization on Karryam Fleet."
	case "STEP_UP":
		return "Confirm a sensitive action", "Use this code to confirm the action you just started in Karryam Fleet."
	case "CONTACT_CHANGE":
		return "Contact change", "Use this code to confirm the change to your contact details."
	default:
		return "Verification", "Use this code to continue in Karryam Fleet."
	}
}

func otpLayout(message OTPMessage) emailbrand.Layout {
	eyebrow, intro := otpPurposeCopy(message.Purpose)
	minutes := int(message.ExpiresIn.Minutes())
	layout := emailbrand.Layout{
		Product: fleetProduct, Eyebrow: eyebrow, Heading: "Your verification code",
		Preheader: "Your Karryam Fleet verification code. Never share it with anyone.",
		Intro:     []string{intro},
		Code:      &emailbrand.Code{Label: "Verification code", Value: message.Code},
		Notice: &emailbrand.Notice{Tone: emailbrand.ToneSecurity, Title: "Keep this code private",
			Text: "Karryam will never ask you for this code by phone, chat or email. If you did not request it, you can safely ignore this email."},
		Support: true,
	}
	if minutes > 0 {
		layout.Code.Expiry = fmt.Sprintf("Expires in %d minutes. It can be used once.", minutes)
	}
	if message.Purpose == "STEP_UP" || message.Purpose == "CONTACT_CHANGE" {
		layout.Notice = &emailbrand.Notice{Tone: emailbrand.ToneWarning, Title: "Didn't start this?",
			Text: "Do not use this code. Sign out of other sessions from your security settings and contact support immediately. Karryam will never ask you for this code."}
	}
	return layout
}

func formatExpiry(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return at.UTC().Format("2 Jan 2006, 15:04 UTC")
}

func greeting(fullName string) string {
	fields := strings.Fields(fullName)
	if len(fields) == 0 {
		return "Hello,"
	}
	return "Hi " + fields[0] + ","
}

func orDefault(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

// invitationLayout returns the layout for staff and provider invitations and
// the provider set-up notice. Callers that supply their own Body keep it as
// the plain-text fallback; the layout is built from the structured fields.
func invitationLayout(message InvitationMessage) emailbrand.Layout {
	organization := orDefault(message.OrganizationName, "your organization")
	expiry := formatExpiry(message.ExpiresAt)
	security := &emailbrand.Notice{Tone: emailbrand.ToneSecurity, Title: "Security notice",
		Text: "Karryam Fleet never uses passwords and will never ask for your verification code. Only accept invitations you were expecting."}

	switch message.EventType {
	case EventCorporateProviderLoginNotice:
		return emailbrand.Layout{
			Product: fleetProduct, Eyebrow: "Provider account",
			Heading:  "Finish setting up your provider account",
			Greeting: greeting(message.RecipientName),
			Intro:    []string{fmt.Sprintf("%s has added you as a provider on Karryam Fleet. Your account is ready for you to finish setting up.", organization)},
			Details: &emailbrand.Card{Title: "Account details", Rows: []emailbrand.Row{
				{Label: "Organization", Value: organization}, {Label: "Sign-in email", Value: message.Email},
			}},
			StepsTitle: "How to finish setting up",
			Steps: []string{"Open your Karryam provider app.", "Sign in with this email address and your phone number.",
				"Enter the one-time code we email you."},
			Notice: security, Support: true,
		}
	case EventCorporateProviderInvitation:
		layout := emailbrand.Layout{
			Product: fleetProduct, Eyebrow: "Provider invitation",
			Heading:    "You're invited to join " + organization,
			Greeting:   greeting(message.RecipientName),
			Intro:      []string{fmt.Sprintf("%s has invited you to work with them as a provider on Karryam Fleet.", organization)},
			Details:    &emailbrand.Card{Title: "Invitation details", Rows: []emailbrand.Row{{Label: "Organization", Value: organization}}},
			StepsTitle: "How to accept",
			Steps: []string{"Select Accept invitation.", "Confirm your email address and phone number.",
				"Enter the one-time code we email you to activate your account."},
			Notice: security, Support: true,
		}
		layout.Details.Rows = appendOptional(layout.Details.Rows, "Invited by", message.InviterName)
		layout.Details.Rows = appendOptional(layout.Details.Rows, "Expires", expiry)
		layout.Action = acceptAction(message.AcceptURL)
		layout.FallbackLink = layout.Action != nil
		return layout
	default:
		layout := emailbrand.Layout{
			Product: fleetProduct, Eyebrow: "Team invitation",
			Heading:    "Join " + organization + " on Karryam Fleet",
			Greeting:   greeting(message.RecipientName),
			Intro:      []string{invitationSentence(organization, message.InviterName)},
			Details:    &emailbrand.Card{Title: "Invitation details", Rows: []emailbrand.Row{{Label: "Organization", Value: organization}}},
			StepsTitle: "How to activate your account",
			Steps: []string{"Select Accept invitation.", "Confirm your email address and phone number.",
				"Enter the one-time code we email you. No password is needed."},
			Notice: security, Support: true,
		}
		if len(message.RoleNames) > 0 {
			label := "Role"
			if len(message.RoleNames) > 1 {
				label = "Roles"
			}
			layout.Details.Rows = append(layout.Details.Rows, emailbrand.Row{Label: label, Value: strings.Join(message.RoleNames, ", ")})
		}
		layout.Details.Rows = appendOptional(layout.Details.Rows, "Invited by", message.InviterName)
		layout.Details.Rows = appendOptional(layout.Details.Rows, "Expires", expiry)
		layout.Action = acceptAction(message.AcceptURL)
		layout.FallbackLink = layout.Action != nil
		if layout.Action == nil {
			layout.Steps[0] = "Ask " + organization + " to resend your invitation link."
		}
		return layout
	}
}

func invitationSentence(organization, inviter string) string {
	if strings.TrimSpace(inviter) != "" {
		return fmt.Sprintf("%s has invited you to join %s on Karryam Fleet, the operations platform your team uses to manage fleet, logistics and staff.", strings.TrimSpace(inviter), organization)
	}
	return fmt.Sprintf("You have been invited to join %s on Karryam Fleet, the operations platform your team uses to manage fleet, logistics and staff.", organization)
}

func appendOptional(rows []emailbrand.Row, label, value string) []emailbrand.Row {
	if strings.TrimSpace(value) == "" {
		return rows
	}
	return append(rows, emailbrand.Row{Label: label, Value: strings.TrimSpace(value)})
}

func acceptAction(url string) *emailbrand.Action {
	if !emailbrand.SafeURL(url) {
		return nil
	}
	return &emailbrand.Action{Label: "Accept invitation", URL: url}
}

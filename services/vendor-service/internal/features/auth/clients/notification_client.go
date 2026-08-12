// Package vendorauthclients adapts vendor-service's AUTHENTICATION OTP
// delivery onto notification-service's existing generic Request/Notifier
// contract (shared/go/notifications) — reused as-is (Section 5's own rule 32,
// Case A: "First inspect whether generic OTP notification infrastructure
// already supports Vendor authentication... reuse it"). No new
// notification-service route, template, or schema change was needed: the
// existing Request already accepts inline title/body, so this never touches
// notification-service's own templates table (see
// docs/PLATFORM_SERVICE_CHANGE_REGISTER.md #3 for the one config-only change
// notification-service itself still needs — allowlisting the
// "vendor-service" HMAC caller identity).
package vendorauthclients

import (
	"context"
	"fmt"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/notifications"
)

const sourceService = "vendor-service"

// OTPNotifier sends the AUTHENTICATION OTP via notification-service. It never
// sends the code through any channel/route associated with the platform's
// logistics pickup/delivery custody handover, and it never logs the code —
// the raw OTP passes through this one call and is discarded by the caller
// immediately after (see internal/features/auth/usecases, which never stores
// or logs it either).
type OTPNotifier struct {
	client notifications.Notifier
}

func NewOTPNotifier(client notifications.Notifier) *OTPNotifier {
	return &OTPNotifier{client: client}
}

// SendOTP implements vendorauthusecases.NotificationSender.
//
// Only email identifiers have a real delivery channel today: verified
// against shared/go/notifications directly — its channel set is exactly
// {push, email, websocket, in_app} (see docs/architecture/00_REPOSITORY_AUDIT_BASELINE.md
// and CLAUDE.md's own "Notification Rules"), there is no SMS channel
// anywhere in the platform. Push and in-app both require an already
// registered device/session, which does not exist for someone who has not
// logged in yet. Phone-identifier OTP delivery is therefore a genuine,
// disclosed external dependency (see
// docs/PLATFORM_SERVICE_CHANGE_REGISTER.md) — this returns Unavailable for
// phone rather than silently pretending to deliver, or (as customer-service's
// existing LoggingOTPSender does) logging the raw code as a fallback, which
// Section 5's own rule 11 forbids outright.
func (n *OTPNotifier) SendOTP(ctx context.Context, identifierType, identifierValue, purpose, otp string) error {
	if n == nil || n.client == nil {
		return fmt.Errorf("vendorauthclients: notification client is not configured")
	}
	if identifierType != "email" {
		return apperrors.Unavailable("Verification code delivery is not yet available for phone numbers. Please use email, or contact your administrator.", nil)
	}

	title := "Your KarryGo Vendor verification code"
	body := fmt.Sprintf("Your KarryGo Vendor verification code is %s. It expires shortly and is for KarryGo Vendor sign-in only — never share it.", otp)
	if purpose == "step_up" {
		title = "Confirm this action"
		body = fmt.Sprintf("Your KarryGo Vendor confirmation code is %s. It expires shortly and confirms a sensitive account action — never share it.", otp)
	}

	_, err := n.client.Send(ctx, notifications.Request{
		IDempotencyKey: notifications.IdempotencyKey(sourceService, "vendor.auth."+purpose+".otp", identifierValue),
		SourceService:  sourceService,
		EventType:      "vendor.auth." + purpose + ".otp",
		Recipient:      notifications.Recipient{Type: "vendor_user", ID: identifierValue, Email: identifierValue},
		Channels:       []string{notifications.ChannelEmail},
		Title:          title,
		Body:           body,
		Priority:       notifications.PriorityHigh,
	})
	return err
}

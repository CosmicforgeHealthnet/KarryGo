// Package clients holds the outbound ports of corporate authentication.
//
// OTP delivery goes through notification-service. There is deliberately no
// logging sender, no console sender and no in-memory sender that reports
// success: an unconfigured or failing transport must surface as
// otp_delivery_unavailable, never as a delivered OTP.
package clients

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cosmicforge/logistics/shared/go/notifications"

	"cosmicforge/logistics/services/fleet-partner-service/internal/config"
)

// Notification event types produced by this service.
const (
	EventCorporateAuthOTP            = "corporate.auth.otp"
	EventCorporateStaffInvitation    = "corporate.staff.invitation"
	EventCorporateProviderInvitation = "corporate.provider.invitation"
	EventCorporateOnboardingUpdate   = "corporate.organization.onboarding_update"
)

// RecipientCorporate is the recipient type for corporate users. The shared
// package currently defines customer and provider; corporate is new here.
const RecipientCorporate = "corporate_user"

// ErrDeliveryUnavailable means the OTP could not be handed to
// notification-service. Callers must surface otp_delivery_unavailable and
// must not claim the code was sent.
var ErrDeliveryUnavailable = errors.New("auth: otp delivery is unavailable")

// OTPMessage is one OTP email to send.
type OTPMessage struct {
	Email          string
	Phone          string
	Code           string
	Purpose        string
	PurposeText    string
	ExpiresIn      time.Duration
	Locale         string
	IdempotencyKey string
	RequestID      string
	CorrelationID  string
}

// InvitationMessage is a staff or provider invitation email. It never carries
// a password or temporary credential — only a link and instructions to
// authenticate with email + phone + emailed OTP.
type InvitationMessage struct {
	Email            string
	Phone            string
	OrganizationName string
	InviterName      string
	InvitationToken  string
	AcceptURL        string
	ExpiresAt        time.Time
	EventType        string
	IdempotencyKey   string
	RequestID        string
	CorrelationID    string
}

// Sender delivers corporate authentication emails.
type Sender interface {
	SendOTP(ctx context.Context, message OTPMessage) (reference string, err error)
	SendInvitation(ctx context.Context, message InvitationMessage) (reference string, err error)
}

// NotificationSender is the production adapter over notification-service.
type NotificationSender struct {
	client notifications.Client
}

// NewNotificationSender builds the adapter. An empty base URL or secret
// yields an adapter whose calls fail with ErrDeliveryUnavailable.
func NewNotificationSender(baseURL string, secret []byte, timeout time.Duration) *NotificationSender {
	return &NotificationSender{
		client: notifications.Client{
			BaseURL:     baseURL,
			ServiceName: config.ServiceName,
			Secret:      secret,
		},
	}
}

// Configured reports whether live delivery is possible.
func (s *NotificationSender) Configured() bool {
	return s != nil && s.client.BaseURL != "" && len(s.client.Secret) > 0
}

// SendOTP delivers the code by email only. No SMS channel is requested.
func (s *NotificationSender) SendOTP(ctx context.Context, message OTPMessage) (string, error) {
	if !s.Configured() {
		return "", fmt.Errorf("%w: notification-service is not configured", ErrDeliveryUnavailable)
	}

	response, err := s.client.Send(ctx, notifications.Request{
		IDempotencyKey: message.IdempotencyKey,
		SourceService:  config.ServiceName,
		EventType:      EventCorporateAuthOTP,
		Recipient: notifications.Recipient{
			Type:  RecipientCorporate,
			ID:    message.Email,
			Email: message.Email,
		},
		// Email only — corporate OTP is never sent by SMS in this release.
		Channels:    []string{notifications.ChannelEmail},
		TemplateKey: "corporate_auth_otp",
		Locale:      message.Locale,
		Title:       "Your KarryGo verification code",
		Priority:    notifications.PriorityHigh,
		TemplateData: map[string]interface{}{
			"code":               message.Code,
			"expires_in_minutes": int(message.ExpiresIn.Minutes()),
			"purpose_text":       message.PurposeText,
		},
		Data: map[string]interface{}{
			"purpose":        message.Purpose,
			"request_id":     message.RequestID,
			"correlation_id": message.CorrelationID,
		},
	})
	if err != nil {
		// The underlying error may name internal hosts; never surface it.
		return "", fmt.Errorf("%w", ErrDeliveryUnavailable)
	}
	return response.MessageID, nil
}

// SendInvitation delivers a staff or provider invitation email.
func (s *NotificationSender) SendInvitation(ctx context.Context, message InvitationMessage) (string, error) {
	if !s.Configured() {
		return "", fmt.Errorf("%w: notification-service is not configured", ErrDeliveryUnavailable)
	}
	eventType := message.EventType
	if eventType == "" {
		eventType = EventCorporateStaffInvitation
	}

	response, err := s.client.Send(ctx, notifications.Request{
		IDempotencyKey: message.IdempotencyKey,
		SourceService:  config.ServiceName,
		EventType:      eventType,
		Recipient: notifications.Recipient{
			Type:  RecipientCorporate,
			ID:    message.Email,
			Email: message.Email,
		},
		Channels:    []string{notifications.ChannelEmail},
		TemplateKey: "corporate_invitation",
		Title:       "You have been invited to " + message.OrganizationName,
		Priority:    notifications.PriorityNormal,
		TemplateData: map[string]interface{}{
			"organization_name": message.OrganizationName,
			"inviter_name":      message.InviterName,
			"accept_url":        message.AcceptURL,
			"expires_at":        message.ExpiresAt.UTC().Format(time.RFC3339),
			// Deliberately no password or temporary credential: the invitee
			// signs in with email + phone + emailed OTP.
			"sign_in_method": "email_and_phone_otp",
		},
		Data: map[string]interface{}{
			"request_id":     message.RequestID,
			"correlation_id": message.CorrelationID,
		},
	})
	if err != nil {
		return "", fmt.Errorf("%w", ErrDeliveryUnavailable)
	}
	return response.MessageID, nil
}

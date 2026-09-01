package securehandover

import (
	"context"
	"errors"
	"strings"

	"cosmicforge/logistics/shared/go/notifications"
)

const (
	eventDeliveryChallenge = "dispatch.delivery_handover_challenge"
	eventHandoverResend    = "dispatch.handover_resend"
	templateDeliveryCode   = "booking.delivery_code_ready"
)

type SecretDeliveryIntent struct {
	Challenge Challenge
	Context   Context
	OTP       string `json:"-"`
	QRToken   string `json:"-"`
	Resend    bool
	Channels  []string
}

type SecretTransport interface {
	Deliver(ctx context.Context, intent SecretDeliveryIntent) (string, string, []string, error)
}

type NotificationTransport struct{ client notifications.Notifier }

func NewNotificationTransport(client notifications.Notifier) *NotificationTransport {
	return &NotificationTransport{client: client}
}

func (n *NotificationTransport) Deliver(ctx context.Context, intent SecretDeliveryIntent) (string, string, []string, error) {
	if n == nil || n.client == nil || len(intent.Channels) == 0 {
		return "", "UNAVAILABLE", nil, errors.New("approved notification channel is unavailable")
	}
	eventType := eventDeliveryChallenge
	if intent.Resend {
		eventType = eventHandoverResend
	}
	recipient := notifications.Recipient{Type: notifications.RecipientCustomer, ID: intent.Context.CustomerID,
		Email: strings.TrimSpace(intent.Context.ReceiverEmail), Phone: strings.TrimSpace(intent.Context.ReceiverPhone)}
	if recipient.ID == "" {
		recipient.Type = "receiver"
		recipient.ID = intent.Challenge.IntendedRecipientRef
	}
	response, err := n.client.Send(ctx, notifications.Request{
		IDempotencyKey:      notifications.IdempotencyKey("driver-dispatch-delivery-service", eventType, intent.Challenge.ChallengeRef+":"+integer(intent.Challenge.Version)),
		SourceService:       "driver-dispatch-delivery-service",
		SourceAggregateType: "handover_challenge",
		SourceAggregateID:   intent.Challenge.ChallengeRef,
		SourceVersion:       intent.Challenge.Version,
		EventType:           eventType,
		Recipient:           recipient,
		Channels:            intent.Channels,
		TemplateKey:         templateDeliveryCode,
		TemplateData: map[string]any{
			"code":              intent.OTP,
			"qr_token":          intent.QRToken,
			"expires_at":        intent.Challenge.ExpiresAt,
			"masked_receiver":   intent.Challenge.MaskedDestination,
			"challenge_version": intent.Challenge.Version,
		},
		Priority: notifications.PriorityHigh,
	})
	if err != nil {
		return "", "FAILED", intent.Channels, err
	}
	return response.MessageID, "ACCEPTED", intent.Channels, nil
}

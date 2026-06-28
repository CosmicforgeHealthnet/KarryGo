package supportclients

import (
	"context"

	supportmodels "cosmicforge/logistics/services/support-dispute-service/internal/features/support/models"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/notifications"
)

const sourceService = "support-dispute-service"

// Support notification event types. Support has no seeded templates, so these
// are sent with inline title/body rather than a template key.
const (
	EventSupportStatusChanged   = "support.status_changed"
	EventSupportMessage         = "support.message"
	EventSupportDisputeResolved = "support.dispute_resolved"
	EventSupportEmergencyAck    = "support.emergency_ack"
)

// SupportNotifier sends support/dispute notifications to the central
// notification-service. It is fire-and-forget: a notification failure must never
// fail a support operation, so send errors are logged, not returned.
type SupportNotifier struct {
	notifier notifications.Notifier
}

// NewSupportNotifier builds a SupportNotifier from the notification-service base
// URL (bare origin) and the shared HMAC secret. When either is empty it returns
// a no-op notifier so the support flow keeps working without notification-service.
func NewSupportNotifier(baseURL string, secret []byte) *SupportNotifier {
	if baseURL == "" || len(secret) == 0 {
		return &SupportNotifier{notifier: noopNotifier{}}
	}
	return &SupportNotifier{notifier: notifications.Client{
		BaseURL:     baseURL,
		ServiceName: sourceService,
		Secret:      secret,
	}}
}

// NewSupportNotifierWith wraps an explicit notifier. Useful in tests.
func NewSupportNotifierWith(notifier notifications.Notifier) *SupportNotifier {
	return &SupportNotifier{notifier: notifier}
}

// recipientTypeFor maps a complainant type to a notification recipient audience
// (customer vs provider — the two audiences notification-service understands).
func recipientTypeFor(t supportmodels.ComplainantType) string {
	if t == supportmodels.ComplainantCustomer {
		return notifications.RecipientCustomer
	}
	return notifications.RecipientProvider
}

// NotifyStatusChanged tells the complainant their complaint status changed.
func (n *SupportNotifier) NotifyStatusChanged(ctx context.Context, c supportmodels.Complaint) {
	n.send(ctx, c.ComplainantType, c.ComplainantID,
		EventSupportStatusChanged, c.ID+":"+string(c.Status),
		"Support update", "Your complaint \""+c.Subject+"\" is now "+string(c.Status)+".",
		map[string]any{"complaint_id": c.ID, "status": string(c.Status)},
		notifications.DefaultChannels, notifications.PriorityNormal)
}

// NotifyDisputeResolved tells the complainant their dispute was resolved.
func (n *SupportNotifier) NotifyDisputeResolved(ctx context.Context, c supportmodels.Complaint, d supportmodels.Dispute) {
	n.send(ctx, c.ComplainantType, c.ComplainantID,
		EventSupportDisputeResolved, d.ID+":"+string(d.Outcome),
		"Dispute resolved", "Your dispute has been resolved: "+string(d.Outcome)+".",
		map[string]any{"complaint_id": c.ID, "dispute_id": d.ID, "outcome": string(d.Outcome)},
		notifications.DefaultChannels, notifications.PriorityNormal)
}

// NotifyNewMessage tells the counterpart a new chat message arrived. When an
// admin replies, the complainant is notified (push + websocket for live chat).
func (n *SupportNotifier) NotifyNewMessage(ctx context.Context, c supportmodels.Complaint, msg supportmodels.ChatMessage) {
	// Only notify the complainant about messages they did not send themselves.
	if string(msg.SenderType) == string(c.ComplainantType) {
		return
	}
	n.send(ctx, c.ComplainantType, c.ComplainantID,
		EventSupportMessage, msg.ID,
		"New support message", msg.Content,
		map[string]any{"complaint_id": c.ID, "message_id": msg.ID, "sender_type": string(msg.SenderType)},
		notifications.DefaultChannels, notifications.PriorityNormal)
}

// NotifyEmergencyAck acknowledges an SOS report to the reporter at high priority.
// (A dedicated admin fan-out channel is a fast-follow: notification-service has
// no admin recipient audience yet — the emergency complaint surfaces at the top
// of the admin queue via its priority sort.)
func (n *SupportNotifier) NotifyEmergencyAck(ctx context.Context, c supportmodels.Complaint) {
	logging.Notice("sos", "emergency complaint id=%s complainant=%s/%s", c.ID, c.ComplainantType, c.ComplainantID)
	n.send(ctx, c.ComplainantType, c.ComplainantID,
		EventSupportEmergencyAck, c.ID,
		"Emergency received", "We've received your emergency report and our team is responding.",
		map[string]any{"complaint_id": c.ID, "priority": c.Priority},
		[]string{notifications.ChannelPush, notifications.ChannelWebSocket, notifications.ChannelInApp},
		notifications.PriorityHigh)
}

func (n *SupportNotifier) send(ctx context.Context, recipientType supportmodels.ComplainantType, recipientID, eventType, dedupeID, title, body string, data map[string]any, channels []string, priority string) {
	if recipientID == "" {
		return
	}
	_, err := n.notifier.Send(ctx, notifications.Request{
		IDempotencyKey: notifications.IdempotencyKey(sourceService, eventType, dedupeID),
		SourceService:  sourceService,
		EventType:      eventType,
		Recipient: notifications.Recipient{
			Type: recipientTypeFor(recipientType),
			ID:   recipientID,
		},
		Channels: channels,
		Title:    title,
		Body:     body,
		Data:     data,
		Priority: priority,
	})
	if err != nil {
		logging.Error("notification", "send event=%s recipient=%s: %v", eventType, recipientID, err)
	}
}

// noopNotifier is used when the notification-service is not configured.
type noopNotifier struct{}

func (noopNotifier) Send(context.Context, notifications.Request) (notifications.SendResponse, error) {
	return notifications.SendResponse{}, nil
}

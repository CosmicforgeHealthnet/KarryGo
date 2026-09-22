package assignment

// Section 14: telling operations about a safety decline.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/marketplace"
	"cosmicforge/logistics/shared/go/notifications"
)

// SafetyDeclineAggregate marks the notification-service row for a safety
// decline. One row per declined offer: the offer id is the fact.
const SafetyDeclineAggregate = "safety_decline"

func emitSafetyDecline(ctx context.Context, tx pgx.Tx, offerID, missionID, providerID, reason string) error {
	raw, _ := json.Marshal(map[string]any{"mission_id": missionID, "provider_id": providerID, "reason_code": reason})
	sum := sha256.Sum256(raw)
	_, err := tx.Exec(ctx, `INSERT INTO dispatch_marketplace_outbox
		(event_type, aggregate_type, aggregate_id, aggregate_version, payload, payload_hash, target_service)
		VALUES ($1, $2, $3, 1, $4, $5, 'notification-service') ON CONFLICT DO NOTHING`,
		notifications.EventNetworkAdminSafetyDecline, SafetyDeclineAggregate, offerID, raw, hex.EncodeToString(sum[:]))
	return err
}

// SafetyDeclineNotifier delivers safety-decline rows to operations and hands
// every other row to Next.
type SafetyDeclineNotifier struct {
	Notifier notifications.Notifier
	Next     marketplace.EventDeliverer
}

func (d SafetyDeclineNotifier) Configured() bool {
	return d.Notifier != nil || (d.Next != nil && d.Next.Configured())
}

func (d SafetyDeclineNotifier) Deliver(ctx context.Context, e marketplace.Envelope) (marketplace.DeliveryResult, error) {
	if e.AggregateType != SafetyDeclineAggregate {
		if d.Next == nil {
			return marketplace.DeliveryResult{Outcome: marketplace.DeliveryRejected, Code: "UNSUPPORTED_NOTIFICATION"}, nil
		}
		return d.Next.Deliver(ctx, e)
	}
	if d.Notifier == nil {
		return marketplace.DeliveryResult{Outcome: marketplace.DeliveryRetryable, Code: "NOTIFICATION_NOT_CONFIGURED"}, nil
	}
	mission, _ := e.Payload["mission_id"].(string)
	provider, _ := e.Payload["provider_id"].(string)
	reason, _ := e.Payload["reason_code"].(string)
	request := notifications.NetworkRequest("driver-dispatch-delivery-service", notifications.NetworkIntent{
		EventType:     notifications.EventNetworkAdminSafetyDecline,
		RecipientType: notifications.RecipientAdminOperations, RecipientID: notifications.AdminOperationsQueue,
		SourceEventID: "safety-decline:" + e.AggregateID, SourceAggregateType: SafetyDeclineAggregate,
		SourceAggregateID: e.AggregateID, SourceVersion: 1, Priority: notifications.PriorityHigh,
		Data: map[string]interface{}{"mission_id": mission, "provider_id": provider, "reason_code": reason},
	})
	response, err := d.Notifier.Send(ctx, request)
	if err != nil {
		return marketplace.DeliveryResult{Outcome: marketplace.DeliveryUnknown, Code: "NOTIFICATION_ACKNOWLEDGEMENT_UNKNOWN"}, err
	}
	if response.MessageID == "" && response.Status == "" {
		return marketplace.DeliveryResult{Outcome: marketplace.DeliveryUnknown, Code: "NOTIFICATION_ACKNOWLEDGEMENT_MISSING"}, nil
	}
	return marketplace.DeliveryResult{Outcome: marketplace.DeliveryAccepted}, nil
}

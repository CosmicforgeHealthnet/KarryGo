package shareworkers

import (
	"context"
	"time"

	r "cosmicforge/logistics/services/customer-service/internal/features/journey_sharing/repositories"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/notifications"
)

type NotificationOutbox interface {
	ClaimNotification(context.Context) (r.NotificationIntent, bool, error)
	AcknowledgeNotification(context.Context, string) error
	RetryNotification(context.Context, r.NotificationIntent) error
}

func Notifications(outbox NotificationOutbox, sender notifications.Notifier, enabled bool) func(context.Context) error {
	return func(ctx context.Context) error {
		if !enabled {
			return nil
		}
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for {
			for i := 0; i < 100 && ctx.Err() == nil; i++ {
				found, err := deliverOne(ctx, outbox, sender)
				if err != nil {
					logging.Structured("error", "journey_share_notification", map[string]any{"outcome": "dependency_error"})
				}
				if !found || err != nil {
					break
				}
			}
			select {
			case <-ctx.Done():
				return nil
			case <-ticker.C:
			}
		}
	}
}

func DeliverOne(ctx context.Context, outbox NotificationOutbox, sender notifications.Notifier) error {
	_, err := deliverOne(ctx, outbox, sender)
	return err
}

func deliverOne(ctx context.Context, outbox NotificationOutbox, sender notifications.Notifier) (bool, error) {
	intent, found, err := outbox.ClaimNotification(ctx)
	if err != nil || !found {
		return false, err
	}
	_, err = sender.Send(ctx, notifications.Request{
		IDempotencyKey:      intent.ID,
		SourceService:       "customer-service",
		SourceAggregateType: "journey_share",
		SourceAggregateID:   intent.SessionID,
		EventType:           intent.EventType,
		Recipient:           notifications.Recipient{Type: notifications.RecipientCustomer, ID: intent.CustomerID},
		TemplateKey:         intent.EventType,
		Data:                map[string]any{"share_ref": intent.SessionID},
	})
	if err != nil {
		if retryErr := outbox.RetryNotification(ctx, intent); retryErr != nil {
			return true, retryErr
		}
		return true, err
	}
	return true, outbox.AcknowledgeNotification(ctx, intent.ID)
}

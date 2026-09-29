package walletclients

import (
	"context"
	"errors"
	"time"

	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/notifications"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type WalletNotificationOutbox struct {
	DB     *pgxpool.Pool
	Sender notifications.Notifier
}

type walletNotificationIntent struct {
	ID, Reference, Event, RecipientType, RecipientID string
	Attempts                                         int
}

func (w WalletNotificationOutbox) DeliverOne(ctx context.Context) (bool, error) {
	var intent walletNotificationIntent
	err := w.DB.QueryRow(ctx, `WITH next AS (
	 SELECT id FROM wallet_notification_outbox WHERE delivered_at IS NULL AND dead_at IS NULL
	 AND available_at<=now() AND (leased_until IS NULL OR leased_until<now())
	 ORDER BY available_at,id LIMIT 1 FOR UPDATE SKIP LOCKED
	) UPDATE wallet_notification_outbox o SET leased_until=now()+interval '30 seconds',attempts=attempts+1
	 FROM next WHERE o.id=next.id
	 RETURNING o.id::text,o.entity_ref,o.event_type,o.recipient_type,o.recipient_id,o.attempts`).Scan(
		&intent.ID, &intent.Reference, &intent.Event, &intent.RecipientType, &intent.RecipientID, &intent.Attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, err = w.Sender.Send(ctx, notifications.Request{
		IDempotencyKey:      notifications.IdempotencyKey(notificationSourceService, intent.Event, intent.Reference),
		SourceService:       notificationSourceService,
		SourceAggregateType: "wallet_operation",
		SourceAggregateID:   intent.Reference,
		EventType:           intent.Event,
		Recipient:           notifications.Recipient{Type: intent.RecipientType, ID: intent.RecipientID},
		TemplateKey:         intent.Event,
		Data:                map[string]any{"reference": intent.Reference},
	})
	if err != nil {
		delay := time.Duration(1<<min(intent.Attempts, 10)) * time.Second
		if delay > time.Hour {
			delay = time.Hour
		}
		_, retryErr := w.DB.Exec(ctx, `UPDATE wallet_notification_outbox SET leased_until=NULL,available_at=now()+$2::interval,
		 dead_at=CASE WHEN attempts>=12 THEN now() ELSE NULL END WHERE id=$1 AND delivered_at IS NULL`, intent.ID, delay.String())
		if retryErr != nil {
			return true, retryErr
		}
		return true, err
	}
	_, err = w.DB.Exec(ctx, `UPDATE wallet_notification_outbox SET delivered_at=now(),leased_until=NULL WHERE id=$1 AND delivered_at IS NULL`, intent.ID)
	return true, err
}

func (w WalletNotificationOutbox) Run(ctx context.Context) error {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		for i := 0; i < 100 && ctx.Err() == nil; i++ {
			found, err := w.DeliverOne(ctx)
			if err != nil {
				logging.Structured("error", "wallet_notification_outbox", map[string]any{"outcome": "dependency_error"})
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

package sharerepositories

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type NotificationIntent struct {
	ID, SessionID, CustomerID, EventType string
	Attempts                             int
}

// ClaimNotification leases one committed hint. A crash after delivery is safe:
// the notification idempotency key is the stable outbox row ID.
func (r *Postgres) ClaimNotification(ctx context.Context) (NotificationIntent, bool, error) {
	var intent NotificationIntent
	err := r.DB.QueryRow(ctx, `WITH next AS (
	 SELECT id FROM journey_share_notification_outbox
	 WHERE delivered_at IS NULL AND dead_at IS NULL AND available_at<=now()
	 AND (leased_until IS NULL OR leased_until<now())
	 ORDER BY available_at,id LIMIT 1 FOR UPDATE SKIP LOCKED
	) UPDATE journey_share_notification_outbox o SET leased_until=now()+interval '30 seconds',attempts=attempts+1
	 FROM next WHERE o.id=next.id
	 RETURNING o.id::text,o.session_id::text,o.customer_id::text,o.event_type,o.attempts`).Scan(&intent.ID, &intent.SessionID, &intent.CustomerID, &intent.EventType, &intent.Attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return NotificationIntent{}, false, nil
	}
	return intent, err == nil, err
}

func (r *Postgres) AcknowledgeNotification(ctx context.Context, id string) error {
	_, err := r.DB.Exec(ctx, `UPDATE journey_share_notification_outbox SET delivered_at=now(),leased_until=NULL WHERE id=$1 AND delivered_at IS NULL`, id)
	return err
}

func (r *Postgres) RetryNotification(ctx context.Context, intent NotificationIntent) error {
	delay := time.Duration(1<<min(intent.Attempts, 10)) * time.Second
	if delay > time.Hour {
		delay = time.Hour
	}
	_, err := r.DB.Exec(ctx, `UPDATE journey_share_notification_outbox SET leased_until=NULL,available_at=now()+$2::interval,
	 dead_at=CASE WHEN attempts>=12 THEN now() ELSE NULL END WHERE id=$1 AND delivered_at IS NULL`, intent.ID, delay.String())
	return err
}

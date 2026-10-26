package admincommands

import (
	"context"
	"time"

	supportmodels "cosmicforge/logistics/services/support-dispute-service/internal/features/support/models"
	"cosmicforge/logistics/shared/go/logging"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MessageNotifier delivers one persisted admin reply to the complainant.
type MessageNotifier interface {
	DeliverNewMessage(ctx context.Context, c supportmodels.Complaint, msg supportmodels.ChatMessage) error
}

// ReplyDelivery notifies complainants about admin replies AFTER the command
// committed: only committed rows are visible to it. The persisted message is
// the truth; this records only whether telling the complainant worked, so a
// notification outage never undoes or duplicates a reply.
type ReplyDelivery struct {
	DB          *pgxpool.Pool
	Notifier    MessageNotifier
	MaxAttempts int
	Interval    time.Duration
}

const defaultMaxAttempts = 8

// RunOnce delivers up to batch due replies and reports how many were sent.
func (d *ReplyDelivery) RunOnce(ctx context.Context, batch int) (int, error) {
	if d.DB == nil || d.Notifier == nil {
		return 0, nil
	}
	if batch <= 0 || batch > 100 {
		batch = 20
	}
	max := d.MaxAttempts
	if max <= 0 {
		max = defaultMaxAttempts
	}
	tx, err := d.DB.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT d.message_id::text, d.attempts, m.sender_type, m.sender_id, m.content, c.id::text, c.complainant_type::text, c.complainant_id
 FROM support_admin_reply_deliveries d
 JOIN support_chat_messages m ON m.id = d.message_id
 JOIN complaints c ON c.id = d.complaint_id
 WHERE d.state = 'PENDING' AND d.next_attempt_at <= now()
 ORDER BY d.next_attempt_at, d.message_id LIMIT $1 FOR UPDATE OF d SKIP LOCKED`, batch)
	if err != nil {
		return 0, err
	}
	type due struct {
		attempts  int
		complaint supportmodels.Complaint
		message   supportmodels.ChatMessage
	}
	var items []due
	for rows.Next() {
		var item due
		var sender, complainant string
		if err := rows.Scan(&item.message.ID, &item.attempts, &sender, &item.message.SenderID, &item.message.Content, &item.complaint.ID, &complainant, &item.complaint.ComplainantID); err != nil {
			rows.Close()
			return 0, err
		}
		item.message.SenderType = supportmodels.SenderType(sender)
		item.message.ComplaintID = item.complaint.ID
		item.complaint.ComplainantType = supportmodels.ComplainantType(complainant)
		items = append(items, item)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	sent := 0
	for _, item := range items {
		sendCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		sendErr := d.Notifier.DeliverNewMessage(sendCtx, item.complaint, item.message)
		cancel()
		if sendErr == nil {
			sent++
			_, err = tx.Exec(ctx, `UPDATE support_admin_reply_deliveries SET state = 'SENT', attempts = attempts + 1, last_attempt_at = now(), delivered_at = now() WHERE message_id = $1`, item.message.ID)
		} else {
			// Identifiers and counts only: never the body or the recipient.
			logging.Error("support-admin-reply", "delivery attempt failed message=%s attempt=%d", item.message.ID, item.attempts+1)
			state := "PENDING"
			if item.attempts+1 >= max {
				state = "FAILED"
			}
			backoff := time.Duration(1<<uint(min(item.attempts, 10))) * 15 * time.Second
			_, err = tx.Exec(ctx, `UPDATE support_admin_reply_deliveries SET state = $2, attempts = attempts + 1, last_attempt_at = now(), next_attempt_at = now() + make_interval(secs => $3) WHERE message_id = $1`, item.message.ID, state, backoff.Seconds())
		}
		if err != nil {
			return sent, err
		}
	}
	return sent, tx.Commit(ctx)
}

// Run is the serviceapp worker loop.
func (d *ReplyDelivery) Run(ctx context.Context) error {
	interval := d.Interval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := d.RunOnce(ctx, 20); err != nil && ctx.Err() == nil {
			logging.Error("support-admin-reply", "delivery pass failed")
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

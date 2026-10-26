// Package campaignrepositories persists campaign revisions and recipients and
// owns their transactional state changes. Every change to a revision's state
// or counts advances its version and appends the Section 22 owner event
// (notification.campaign.changed.v1) in the same transaction.
package campaignrepositories

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	campaignmodels "cosmicforge/logistics/services/notification-service/internal/features/campaigns/models"
	messagerepositories "cosmicforge/logistics/services/notification-service/internal/features/messages/repositories"
	"cosmicforge/logistics/shared/go/admincampaign"
	"cosmicforge/logistics/shared/go/adminevents"
)

var (
	ErrNotFound = errors.New("campaign revision not found")
	ErrConflict = admincampaign.ErrConflict
	// ErrInactive means the revision can no longer send (cancelled or done).
	ErrInactive = errors.New("campaign revision is not active")
)

type Repository struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

const revisionColumns = `campaign_id::text,revision,intent,intent_digest,idempotency_key,state,rejection_code,content_title,content_body,
 plan_cursor,planned_at,audience_count,eligible_count,suppressed_count,queued_count,sent_count,failed_count,logged_only_count,
 suppression_reasons,version,updated_at`

type rowScanner interface{ Scan(dest ...any) error }

func scanRevision(row rowScanner) (campaignmodels.Revision, error) {
	var r campaignmodels.Revision
	var intent, cursor, reasons []byte
	err := row.Scan(&r.CampaignID, &r.Revision, &intent, &r.IntentDigest, &r.IdempotencyKey, &r.State, &r.RejectionCode, &r.ContentTitle, &r.ContentBody,
		&cursor, &r.PlannedAt, &r.Counts.Audience, &r.Counts.Eligible, &r.Counts.Suppressed, &r.Counts.Queued, &r.Counts.Sent, &r.Counts.Failed, &r.Counts.LoggedOnly,
		&reasons, &r.Version, &r.UpdatedAt)
	if err != nil {
		return r, err
	}
	if err := json.Unmarshal(intent, &r.Intent); err != nil {
		return r, err
	}
	_ = json.Unmarshal(cursor, &r.PlanCursor)
	r.Counts.Reasons = map[string]int64{}
	_ = json.Unmarshal(reasons, &r.Counts.Reasons)
	return r, nil
}

// Accept stores a validated intent as an immutable revision. The same
// idempotency key with the same digest, or the same (campaign, revision) with
// the same digest, returns the existing revision. Any other reuse, or a
// revision lower than one already accepted, is a conflict. A higher revision
// cancels the unsent work of every older revision of the campaign.
func (r *Repository) Accept(ctx context.Context, intent admincampaign.Intent, digest string) (campaignmodels.Revision, bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return campaignmodels.Revision{}, false, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('campaign:'||$1,0))`, intent.CampaignID); err != nil {
		return campaignmodels.Revision{}, false, err
	}
	existing, err := scanRevision(tx.QueryRow(ctx, `SELECT `+revisionColumns+` FROM campaigns WHERE idempotency_key=$1`, intent.IdempotencyKey))
	if err == nil {
		if existing.IntentDigest != digest || existing.CampaignID != intent.CampaignID || existing.Revision != intent.Revision {
			return campaignmodels.Revision{}, false, ErrConflict
		}
		return existing, false, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return campaignmodels.Revision{}, false, err
	}
	existing, err = scanRevision(tx.QueryRow(ctx, `SELECT `+revisionColumns+` FROM campaigns WHERE campaign_id=$1 AND revision=$2`, intent.CampaignID, intent.Revision))
	if err == nil {
		if existing.IntentDigest != digest {
			return campaignmodels.Revision{}, false, ErrConflict
		}
		return existing, false, tx.Commit(ctx)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return campaignmodels.Revision{}, false, err
	}
	var highest int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(revision),0) FROM campaigns WHERE campaign_id=$1`, intent.CampaignID).Scan(&highest); err != nil {
		return campaignmodels.Revision{}, false, err
	}
	if highest > intent.Revision {
		return campaignmodels.Revision{}, false, ErrConflict
	}
	rawIntent, err := json.Marshal(intent)
	if err != nil {
		return campaignmodels.Revision{}, false, err
	}
	approvals, err := json.Marshal(intent.Approvals)
	if err != nil {
		return campaignmodels.Revision{}, false, err
	}
	created, err := scanRevision(tx.QueryRow(ctx, `INSERT INTO campaigns(campaign_id,revision,intent,intent_digest,idempotency_key,state,requested_by,approvals,next_run_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,now()) RETURNING `+revisionColumns,
		intent.CampaignID, intent.Revision, rawIntent, digest, intent.IdempotencyKey, campaignmodels.StateAccepted, intent.RequestedBy, approvals))
	if err != nil {
		return campaignmodels.Revision{}, false, err
	}
	if err := emit(ctx, tx, created); err != nil {
		return campaignmodels.Revision{}, false, err
	}
	// Supersede older revisions: nothing unsent from them may go out.
	rows, err := tx.Query(ctx, `SELECT revision FROM campaigns WHERE campaign_id=$1 AND revision<$2 AND state = ANY($3) ORDER BY revision`, intent.CampaignID, intent.Revision, campaignmodels.ActiveStates)
	if err != nil {
		return campaignmodels.Revision{}, false, err
	}
	var older []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return campaignmodels.Revision{}, false, err
		}
		older = append(older, v)
	}
	rows.Close()
	for _, v := range older {
		if err := cancelTx(ctx, tx, intent.CampaignID, v); err != nil {
			return campaignmodels.Revision{}, false, err
		}
	}
	return created, true, tx.Commit(ctx)
}

// Get returns one revision.
func (r *Repository) Get(ctx context.Context, campaignID string, revision int64) (campaignmodels.Revision, error) {
	rev, err := scanRevision(r.db.QueryRow(ctx, `SELECT `+revisionColumns+` FROM campaigns WHERE campaign_id=$1 AND revision=$2`, campaignID, revision))
	if errors.Is(err, pgx.ErrNoRows) {
		return rev, ErrNotFound
	}
	return rev, err
}

// Cancel stops all unsent work of a revision. It is idempotent by
// cancellation key; a key reused for another revision is a conflict. Sent
// messages and their history are kept.
func (r *Repository) Cancel(ctx context.Context, c admincampaign.Cancellation) (campaignmodels.Revision, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return campaignmodels.Revision{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('campaign:'||$1,0))`, c.CampaignID); err != nil {
		return campaignmodels.Revision{}, err
	}
	var boundID string
	var boundRev int64
	err = tx.QueryRow(ctx, `SELECT campaign_id::text,revision FROM campaign_cancellations WHERE cancellation_key=$1`, c.CancellationKey).Scan(&boundID, &boundRev)
	if err == nil {
		if boundID != c.CampaignID || boundRev != c.Revision {
			return campaignmodels.Revision{}, ErrConflict
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return campaignmodels.Revision{}, err
	} else {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM campaigns WHERE campaign_id=$1 AND revision=$2)`, c.CampaignID, c.Revision).Scan(&exists); err != nil {
			return campaignmodels.Revision{}, err
		}
		if !exists {
			return campaignmodels.Revision{}, ErrNotFound
		}
		if _, err := tx.Exec(ctx, `INSERT INTO campaign_cancellations(cancellation_key,campaign_id,revision,requested_by,reason_code) VALUES($1,$2,$3,$4,$5)`,
			c.CancellationKey, c.CampaignID, c.Revision, c.RequestedBy, c.ReasonCode); err != nil {
			return campaignmodels.Revision{}, err
		}
		if err := cancelTx(ctx, tx, c.CampaignID, c.Revision); err != nil {
			return campaignmodels.Revision{}, err
		}
	}
	rev, err := scanRevision(tx.QueryRow(ctx, `SELECT `+revisionColumns+` FROM campaigns WHERE campaign_id=$1 AND revision=$2`, c.CampaignID, c.Revision))
	if err != nil {
		return rev, err
	}
	return rev, tx.Commit(ctx)
}

// cancelTx cancels planned recipients, withdraws not-yet-attempted queued
// deliveries from the pipeline, and marks a non-terminal revision CANCELLED.
func cancelTx(ctx context.Context, tx pgx.Tx, campaignID string, revision int64) error {
	var state string
	if err := tx.QueryRow(ctx, `SELECT state FROM campaigns WHERE campaign_id=$1 AND revision=$2 FOR UPDATE`, campaignID, revision).Scan(&state); err != nil {
		return err
	}
	if campaignmodels.Terminal(state) {
		return nil
	}
	if _, err := tx.Exec(ctx, `UPDATE campaign_recipients SET state='CANCELLED',updated_at=now() WHERE campaign_id=$1 AND revision=$2 AND state='PLANNED'`, campaignID, revision); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `UPDATE notification_deliveries d SET status='suppressed',last_error='campaign_cancelled',updated_at=now()
		FROM campaign_recipients r WHERE r.campaign_id=$1 AND r.revision=$2 AND r.state='QUEUED' AND d.message_id=r.message_id AND d.status IN ('queued','retrying')
		RETURNING d.message_id::text`, campaignID, revision)
	if err != nil {
		return err
	}
	var messages []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		messages = append(messages, id)
	}
	rows.Close()
	for _, id := range messages {
		if _, err := tx.Exec(ctx, messagerepositories.RefreshMessageStatusSQL, id); err != nil {
			return err
		}
	}
	if err := reconcileTx(ctx, tx, campaignID, revision); err != nil {
		return err
	}
	return refreshTx(ctx, tx, campaignID, revision, campaignmodels.StateCancelled, nil)
}

// ClaimDue leases up to limit active revisions whose next run is due.
func (r *Repository) ClaimDue(ctx context.Context, owner string, limit int, lease time.Duration) ([]campaignmodels.Revision, error) {
	rows, err := r.db.Query(ctx, `UPDATE campaigns SET lease_owner=$1, lease_expires_at=now()+$2::interval
		WHERE (campaign_id,revision) IN (SELECT campaign_id,revision FROM campaigns WHERE state = ANY($3) AND next_run_at<=now()
		  AND (lease_expires_at IS NULL OR lease_expires_at<now()) ORDER BY next_run_at LIMIT $4 FOR UPDATE SKIP LOCKED)
		RETURNING `+revisionColumns, owner, fmt.Sprintf("%d milliseconds", lease.Milliseconds()), campaignmodels.ActiveStates, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []campaignmodels.Revision
	for rows.Next() {
		rev, err := scanRevision(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, rev)
	}
	return out, rows.Err()
}

// Release ends a lease and schedules the next run.
func (r *Repository) Release(ctx context.Context, campaignID string, revision int64, owner string, next time.Time) error {
	_, err := r.db.Exec(ctx, `UPDATE campaigns SET lease_owner=NULL,lease_expires_at=NULL,next_run_at=$4 WHERE campaign_id=$1 AND revision=$2 AND lease_owner=$3`,
		campaignID, revision, owner, next)
	return err
}

// Reject records a terminal rejection.
func (r *Repository) Reject(ctx context.Context, campaignID string, revision int64, code string) error {
	return r.inTx(ctx, func(tx pgx.Tx) error {
		return refreshTx(ctx, tx, campaignID, revision, campaignmodels.StateRejected, &code)
	})
}

// RecordContent stores the template copy validated against the intent's
// revision and hash. Sends use exactly this copy.
func (r *Repository) RecordContent(ctx context.Context, campaignID string, revision int64, title, body string) error {
	_, err := r.db.Exec(ctx, `UPDATE campaigns SET content_title=$3,content_body=$4 WHERE campaign_id=$1 AND revision=$2 AND state='ACCEPTED_BY_KARRYGO'`, campaignID, revision, title, body)
	return err
}

// Template is the active template a campaign references.
type Template struct {
	Title, Body string
	Revision    int64
	Active      bool
}

// GetTemplate reads one template by exact key and locale (no fallback).
func (r *Repository) GetTemplate(ctx context.Context, key, locale string) (Template, bool, error) {
	var t Template
	err := r.db.QueryRow(ctx, `SELECT title,body,revision,active FROM notification_templates WHERE key=$1 AND locale=$2`, key, locale).Scan(&t.Title, &t.Body, &t.Revision, &t.Active)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, false, nil
	}
	return t, err == nil, err
}

// PlannedMember is one audience member to record.
type PlannedMember struct {
	RecipientID, Channel, Market string
	ConsentVersion               int64
}

// RecordPlan inserts audience members (deduplicated per recipient and
// channel) and advances the plan cursor. When done, planning completes.
func (r *Repository) RecordPlan(ctx context.Context, campaignID string, revision int64, members []PlannedMember, cursor campaignmodels.PlanCursor, done bool) error {
	return r.inTx(ctx, func(tx pgx.Tx) error {
		var state string
		if err := tx.QueryRow(ctx, `SELECT state FROM campaigns WHERE campaign_id=$1 AND revision=$2 FOR UPDATE`, campaignID, revision).Scan(&state); err != nil {
			return err
		}
		if state != campaignmodels.StateAccepted {
			return ErrInactive
		}
		for _, m := range members {
			if _, err := tx.Exec(ctx, `INSERT INTO campaign_recipients(campaign_id,revision,recipient_id,channel,market,consent_version,state)
				VALUES($1,$2,$3,$4,$5,$6,'PLANNED') ON CONFLICT (campaign_id,revision,recipient_id,channel) DO NOTHING`,
				campaignID, revision, m.RecipientID, m.Channel, m.Market, m.ConsentVersion); err != nil {
				return err
			}
		}
		raw, _ := json.Marshal(cursor)
		if _, err := tx.Exec(ctx, `UPDATE campaigns SET plan_cursor=$3 WHERE campaign_id=$1 AND revision=$2`, campaignID, revision, raw); err != nil {
			return err
		}
		if !done {
			return refreshTx(ctx, tx, campaignID, revision, "", nil)
		}
		if _, err := tx.Exec(ctx, `UPDATE campaigns SET planned_at=now() WHERE campaign_id=$1 AND revision=$2`, campaignID, revision); err != nil {
			return err
		}
		return refreshTx(ctx, tx, campaignID, revision, campaignmodels.StatePlanned, nil)
	})
}

// PlannedRecipient is a recipient awaiting its send-time checks.
type PlannedRecipient struct {
	RecipientID, Channel, Market string
	ConsentVersion               int64
}

// ListPlanned returns planned recipients of one channel in id order.
func (r *Repository) ListPlanned(ctx context.Context, campaignID string, revision int64, channel string, limit int) ([]PlannedRecipient, error) {
	rows, err := r.db.Query(ctx, `SELECT recipient_id,channel,market,consent_version FROM campaign_recipients
		WHERE campaign_id=$1 AND revision=$2 AND channel=$3 AND state='PLANNED' ORDER BY recipient_id LIMIT $4`, campaignID, revision, channel, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PlannedRecipient
	for rows.Next() {
		var p PlannedRecipient
		if err := rows.Scan(&p.RecipientID, &p.Channel, &p.Market, &p.ConsentVersion); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// Suppress marks still-planned recipients SUPPRESSED with a reason.
func (r *Repository) Suppress(ctx context.Context, campaignID string, revision int64, channel string, recipients []string, reason string) error {
	if len(recipients) == 0 {
		return nil
	}
	_, err := r.db.Exec(ctx, `UPDATE campaign_recipients SET state='SUPPRESSED',suppression_reason=$5,updated_at=now()
		WHERE campaign_id=$1 AND revision=$2 AND channel=$3 AND recipient_id = ANY($4) AND state='PLANNED'`, campaignID, revision, channel, recipients, reason)
	return err
}

// SuppressAllPlanned suppresses every remaining planned recipient.
func (r *Repository) SuppressAllPlanned(ctx context.Context, campaignID string, revision int64, reason string) error {
	_, err := r.db.Exec(ctx, `UPDATE campaign_recipients SET state='SUPPRESSED',suppression_reason=$3,updated_at=now()
		WHERE campaign_id=$1 AND revision=$2 AND state='PLANNED'`, campaignID, revision, reason)
	return err
}

// EnqueueLimits are the caps evaluated under the recipient lock.
type EnqueueLimits struct {
	MaxPerRecipientPerDay int64
	MaxMessages           int64
}

// EnqueueOutcome reports what happened to one recipient.
type EnqueueOutcome struct {
	Queued         bool
	SuppressReason string
	MessageID      string
}

// Enqueue applies the send-time caps and channel preference to one planned
// recipient and, only if all pass, calls send (which must be idempotent) and
// records QUEUED. It holds a share lock on the revision so a concurrent
// cancellation either precedes it (nothing is sent) or waits for it (the
// queued delivery is then withdrawn). The frequency cap counts messages queued
// for the recipient in the last 24 hours across ALL campaigns, serialized per
// recipient so concurrent campaigns cannot both pass the cap.
func (r *Repository) Enqueue(ctx context.Context, campaignID string, revision int64, recipient, channel string, limits EnqueueLimits, send func(context.Context) (string, error)) (EnqueueOutcome, error) {
	var out EnqueueOutcome
	err := r.inTx(ctx, func(tx pgx.Tx) error {
		var state string
		if err := tx.QueryRow(ctx, `SELECT state FROM campaigns WHERE campaign_id=$1 AND revision=$2 FOR SHARE`, campaignID, revision).Scan(&state); err != nil {
			return err
		}
		if campaignmodels.Terminal(state) {
			return ErrInactive
		}
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('campaign-recipient:'||$1,0))`, recipient); err != nil {
			return err
		}
		var current string
		if err := tx.QueryRow(ctx, `SELECT state FROM campaign_recipients WHERE campaign_id=$1 AND revision=$2 AND recipient_id=$3 AND channel=$4 FOR UPDATE`,
			campaignID, revision, recipient, channel).Scan(&current); err != nil {
			return err
		}
		if current != campaignmodels.RecipientPlanned {
			return nil
		}
		suppress := func(reason string) error {
			out.SuppressReason = reason
			_, err := tx.Exec(ctx, `UPDATE campaign_recipients SET state='SUPPRESSED',suppression_reason=$5,updated_at=now() WHERE campaign_id=$1 AND revision=$2 AND recipient_id=$3 AND channel=$4`,
				campaignID, revision, recipient, channel, reason)
			return err
		}
		var given int64
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM campaign_recipients WHERE campaign_id=$1 AND revision=$2 AND queued_at IS NOT NULL`, campaignID, revision).Scan(&given); err != nil {
			return err
		}
		if given >= limits.MaxMessages {
			return suppress(campaignmodels.SuppressCampaignCap)
		}
		var recent int64
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM campaign_recipients WHERE recipient_id=$1 AND queued_at > now() - interval '24 hours'`, recipient).Scan(&recent); err != nil {
			return err
		}
		if recent >= limits.MaxPerRecipientPerDay {
			return suppress(campaignmodels.SuppressFrequencyCap)
		}
		enabled := true
		err := tx.QueryRow(ctx, `SELECT enabled FROM notification_preferences WHERE recipient_type=$1 AND recipient_id=$2 AND channel=$3`, campaignmodels.RecipientType, recipient, channel).Scan(&enabled)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if !enabled {
			return suppress(campaignmodels.SuppressChannelDisabled)
		}
		messageID, err := send(ctx)
		if err != nil {
			return err
		}
		out.Queued, out.MessageID = true, messageID
		_, err = tx.Exec(ctx, `UPDATE campaign_recipients SET state='QUEUED',message_id=$5,queued_at=now(),updated_at=now() WHERE campaign_id=$1 AND revision=$2 AND recipient_id=$3 AND channel=$4`,
			campaignID, revision, recipient, channel, messageID)
		return err
	})
	return out, err
}

// Reconcile maps pipeline delivery outcomes onto recipients and refreshes
// the revision's counts and state.
func (r *Repository) Reconcile(ctx context.Context, campaignID string, revision int64) error {
	return r.inTx(ctx, func(tx pgx.Tx) error {
		if err := reconcileTx(ctx, tx, campaignID, revision); err != nil {
			return err
		}
		return refreshTx(ctx, tx, campaignID, revision, "", nil)
	})
}

// reconcileTx: a real provider acceptance is SENT; a development logging
// sender is LOGGED_ONLY (never sent); failed/dead-lettered is FAILED; a
// pipeline suppression or skip becomes a recipient suppression with its
// reason. Pending/retrying deliveries leave the recipient QUEUED. A cancelled
// recipient changes only if its delivery actually went out.
func reconcileTx(ctx context.Context, tx pgx.Tx, campaignID string, revision int64) error {
	_, err := tx.Exec(ctx, `
WITH outcome AS (
 SELECT r.recipient_id, r.channel, r.state AS prior,
  CASE
   WHEN d.status='sent' AND (last.provider IS NULL OR last.provider LIKE 'logging\_%') THEN 'LOGGED_ONLY'
   WHEN d.status='sent' THEN 'SENT'
   WHEN r.state='CANCELLED' THEN NULL
   WHEN d.status IN ('failed','dead_lettered') THEN 'FAILED'
   WHEN d.status='suppressed' AND d.last_error='campaign_cancelled' THEN 'CANCELLED'
   WHEN d.status='suppressed' THEN 'SUPPRESSED'
   WHEN d.status='skipped' AND d.last_error LIKE '%no active push devices%' THEN 'SUPPRESSED'
   WHEN d.status='skipped' AND d.last_error LIKE '%email is missing%' THEN 'SUPPRESSED'
   WHEN d.status='skipped' THEN 'FAILED'
  END AS next_state,
  CASE
   WHEN d.status='suppressed' AND d.last_error IS DISTINCT FROM 'campaign_cancelled' THEN 'CHANNEL_DISABLED'
   WHEN d.status='skipped' AND d.last_error LIKE '%no active push devices%' THEN 'NO_ACTIVE_DEVICE'
   WHEN d.status='skipped' AND d.last_error LIKE '%email is missing%' THEN 'NO_CONTACT_ADDRESS'
  END AS reason
 FROM campaign_recipients r
 JOIN notification_deliveries d ON d.message_id=r.message_id AND d.channel=r.channel
 -- The provider that produced the final outcome is recorded per attempt.
 LEFT JOIN LATERAL (SELECT a.provider FROM notification_delivery_attempts a WHERE a.delivery_id=d.id ORDER BY a.id DESC LIMIT 1) last ON true
 WHERE r.campaign_id=$1 AND r.revision=$2 AND r.state IN ('QUEUED','CANCELLED') AND r.message_id IS NOT NULL
)
UPDATE campaign_recipients r SET state=o.next_state, suppression_reason=CASE WHEN o.next_state='SUPPRESSED' THEN o.reason END, updated_at=now()
FROM outcome o
WHERE r.campaign_id=$1 AND r.revision=$2 AND r.recipient_id=o.recipient_id AND r.channel=o.channel
  AND o.next_state IS NOT NULL AND o.next_state <> o.prior`, campaignID, revision)
	return err
}

// refreshTx recomputes counts and state. override forces a state (e.g.
// CANCELLED, REJECTED, PLANNED at the end of planning); otherwise the state
// is derived. Any change advances the version and emits the owner event.
func refreshTx(ctx context.Context, tx pgx.Tx, campaignID string, revision int64, override string, rejection *string) error {
	current, err := scanRevision(tx.QueryRow(ctx, `SELECT `+revisionColumns+` FROM campaigns WHERE campaign_id=$1 AND revision=$2 FOR UPDATE`, campaignID, revision))
	if err != nil {
		return err
	}
	var c campaignmodels.Counts
	if err := tx.QueryRow(ctx, `SELECT count(*),
		count(*) FILTER (WHERE state IN ('QUEUED','SENT','LOGGED_ONLY','FAILED')),
		count(*) FILTER (WHERE state='SUPPRESSED'), count(*) FILTER (WHERE state='QUEUED'),
		count(*) FILTER (WHERE state='SENT'), count(*) FILTER (WHERE state='FAILED'),
		count(*) FILTER (WHERE state='LOGGED_ONLY'), count(*) FILTER (WHERE state='PLANNED'),
		count(*) FILTER (WHERE state='CANCELLED')
		FROM campaign_recipients WHERE campaign_id=$1 AND revision=$2`, campaignID, revision).Scan(
		&c.Audience, &c.Eligible, &c.Suppressed, &c.Queued, &c.Sent, &c.Failed, &c.LoggedOnly, &c.Planned, &c.Cancelled); err != nil {
		return err
	}
	c.Reasons = map[string]int64{}
	rows, err := tx.Query(ctx, `SELECT suppression_reason,count(*) FROM campaign_recipients WHERE campaign_id=$1 AND revision=$2 AND state='SUPPRESSED' GROUP BY suppression_reason`, campaignID, revision)
	if err != nil {
		return err
	}
	for rows.Next() {
		var reason string
		var n int64
		if err := rows.Scan(&reason, &n); err != nil {
			rows.Close()
			return err
		}
		c.Reasons[reason] = n
	}
	rows.Close()
	state := current.State
	switch {
	case campaignmodels.Terminal(current.State):
		// A terminal revision's state never changes; counts may still settle
		// (e.g. a delivery that was already in flight when cancelled).
	case override != "":
		state = override
	case current.State == campaignmodels.StateAccepted:
	default:
		state = campaignmodels.ResolveState(current.State, c)
	}
	if override == campaignmodels.StatePlanned && !campaignmodels.Terminal(current.State) {
		state = campaignmodels.ResolveState(campaignmodels.StatePlanned, c)
	}
	var code *string
	if state == campaignmodels.StateRejected {
		code = current.RejectionCode
		if code == nil {
			code = rejection
		}
	}
	same := state == current.State && c.Audience == current.Counts.Audience && c.Eligible == current.Counts.Eligible &&
		c.Suppressed == current.Counts.Suppressed && c.Queued == current.Counts.Queued && c.Sent == current.Counts.Sent &&
		c.Failed == current.Counts.Failed && c.LoggedOnly == current.Counts.LoggedOnly && reflect.DeepEqual(c.Reasons, current.Counts.Reasons)
	if same {
		return nil
	}
	reasons, _ := json.Marshal(c.Reasons)
	updated, err := scanRevision(tx.QueryRow(ctx, `UPDATE campaigns SET state=$3,rejection_code=$4,audience_count=$5,eligible_count=$6,suppressed_count=$7,
		queued_count=$8,sent_count=$9,failed_count=$10,logged_only_count=$11,suppression_reasons=$12,version=version+1,updated_at=now()
		WHERE campaign_id=$1 AND revision=$2 RETURNING `+revisionColumns,
		campaignID, revision, state, code, c.Audience, c.Eligible, c.Suppressed, c.Queued, c.Sent, c.Failed, c.LoggedOnly, reasons))
	if err != nil {
		return err
	}
	return emit(ctx, tx, updated)
}

// Event builds the owner event for a revision's current state.
func Event(rev campaignmodels.Revision) (adminevents.SourceEvent, error) {
	var rejection any
	if rev.RejectionCode != nil {
		rejection = *rev.RejectionCode
	}
	payload := map[string]any{
		"campaign_id": rev.CampaignID, "revision": rev.Revision, "state": rev.State, "rejection_code": rejection,
		"audience_count": rev.Counts.Audience, "eligible_count": rev.Counts.Eligible, "suppressed_count": rev.Counts.Suppressed,
		"queued_count": rev.Counts.Queued, "sent_count": rev.Counts.Sent, "failed_count": rev.Counts.Failed, "logged_only_count": rev.Counts.LoggedOnly,
		"delivered_count": nil, "delivery_confirmation": campaignmodels.DeliveryConfirmationUnavailable, "updated_at": rev.UpdatedAt.UTC(),
	}
	scope := adminevents.Scope{}
	if len(rev.Intent.Markets) == 1 {
		scope.Market = rev.Intent.Markets[0]
	}
	return adminevents.NewSourceEvent(campaignmodels.SourceService, adminevents.NotificationCampaignStat, "campaign_revision",
		rev.CampaignID+":"+strconv.FormatInt(rev.Revision, 10), rev.Version, rev.UpdatedAt, scope, rev.Intent.CorrelationID, payload, false)
}

func emit(ctx context.Context, tx pgx.Tx, rev campaignmodels.Revision) error {
	ev, err := Event(rev)
	if err != nil {
		return err
	}
	return adminevents.Emit(ctx, tx, ev)
}

// Snapshot pages every revision in (campaign_id, revision) order after the
// opaque cursor "<campaign_id>:<revision>".
func (r *Repository) Snapshot(ctx context.Context, cursor string, limit int) ([]campaignmodels.Revision, string, time.Time, error) {
	afterID, afterRev := "", int64(0)
	if cursor != "" {
		id, rev, ok := strings.Cut(cursor, ":")
		n, err := strconv.ParseInt(rev, 10, 64)
		if !ok || err != nil || n < 1 || len(id) != 36 {
			return nil, "", time.Time{}, ErrConflict
		}
		afterID, afterRev = id, n
	}
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly, IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return nil, "", time.Time{}, err
	}
	defer tx.Rollback(ctx)
	var watermark time.Time
	if err := tx.QueryRow(ctx, `SELECT now()`).Scan(&watermark); err != nil {
		return nil, "", time.Time{}, err
	}
	rows, err := tx.Query(ctx, `SELECT `+revisionColumns+` FROM campaigns WHERE (campaign_id::text,revision) > ($1,$2) ORDER BY campaign_id::text,revision LIMIT $3`, afterID, afterRev, limit+1)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	defer rows.Close()
	var out []campaignmodels.Revision
	for rows.Next() {
		rev, err := scanRevision(rows)
		if err != nil {
			return nil, "", time.Time{}, err
		}
		out = append(out, rev)
	}
	if err := rows.Err(); err != nil {
		return nil, "", time.Time{}, err
	}
	next := ""
	if len(out) > limit {
		out = out[:limit]
		last := out[len(out)-1]
		next = last.CampaignID + ":" + strconv.FormatInt(last.Revision, 10)
	}
	return out, next, watermark.UTC(), nil
}

// ConsumeNonce records a single-use request nonce. A repeat returns false.
func (r *Repository) ConsumeNonce(ctx context.Context, caller, nonce string) (bool, error) {
	sum := sha256.Sum256([]byte(nonce))
	tag, err := r.db.Exec(ctx, `INSERT INTO campaign_request_nonces(caller,nonce_hash) VALUES($1,$2) ON CONFLICT DO NOTHING`, caller, hex.EncodeToString(sum[:]))
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() == 1, nil
}

// PruneNonces deletes nonces older than the signature freshness window.
func (r *Repository) PruneNonces(ctx context.Context, olderThan time.Duration) error {
	_, err := r.db.Exec(ctx, `DELETE FROM campaign_request_nonces WHERE seen_at < now() - $1::interval`, fmt.Sprintf("%d seconds", int64(olderThan.Seconds())))
	return err
}

func (r *Repository) inTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

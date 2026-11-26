// Package repositories persists the Section 23 communication records. Every
// statement is tenant-scoped by organization ID. Methods taking a
// database.Querier run inside the caller's transaction; nil means the pool.
package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/communications/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Repository is the PostgreSQL communication store.
type Repository struct{ pool *pgxpool.Pool }

// NewRepository builds the store.
func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (r *Repository) q(q database.Querier) database.Querier {
	if q == nil {
		return r.pool
	}
	return q
}

// WithinTx runs f in one transaction.
func (r *Repository) WithinTx(ctx context.Context, f func(pgx.Tx) error) error {
	return database.WithinTx(ctx, r.pool, f)
}

func jsonRow(row pgx.Row) (map[string]any, error) {
	var raw []byte
	if e := row.Scan(&raw); e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return nil, apperrors.NotFound("Resource not found.", nil)
		}
		return nil, e
	}
	var out map[string]any
	if e := json.Unmarshal(raw, &out); e != nil {
		return nil, e
	}
	return out, nil
}

func jsonList(rows pgx.Rows) ([]map[string]any, error) {
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var raw []byte
		if e := rows.Scan(&raw); e != nil {
			return nil, e
		}
		var v map[string]any
		if e := json.Unmarshal(raw, &v); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func scanStrings(rows pgx.Rows) ([]string, error) {
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Threads
// ---------------------------------------------------------------------------

// InsertThread creates a thread row.
func (r *Repository) InsertThread(ctx context.Context, q database.Querier, id, organizationID, actorID string, in models.ThreadInput) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_communication_threads(id,organization_id,branch_id,subject_type,subject_reference,title,visibility,created_by,owner_user_id) VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9)`, id, organizationID, in.BranchID, in.SubjectType, in.SubjectReference, in.Title, in.Visibility, actorID, in.OwnerUserID)
	return err
}

// InsertParticipant adds one participant to a thread.
func (r *Repository) InsertParticipant(ctx context.Context, q database.Querier, threadID, organizationID, actorID string, p models.ParticipantInput) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_communication_participants(thread_id,organization_id,participant_type,participant_reference,access_level,added_by) VALUES($1,$2,$3,$4,$5,$6)`, threadID, organizationID, p.ParticipantType, p.ParticipantReference, p.AccessLevel, actorID)
	return err
}

// IsUserParticipant reports whether the user takes part in the thread.
func (r *Repository) IsUserParticipant(ctx context.Context, organizationID, threadID, userID string) (bool, error) {
	var ok bool
	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM fleet_partner_communication_participants WHERE organization_id=$1 AND thread_id=$2 AND participant_type='USER' AND participant_reference=$3)`, organizationID, threadID, userID).Scan(&ok)
	return ok, err
}

// GetThread reads a thread with its participants.
func (r *Repository) GetThread(ctx context.Context, organizationID, id string) (map[string]any, error) {
	return jsonRow(r.pool.QueryRow(ctx, `SELECT row_to_json(x) FROM (SELECT th.*,COALESCE((SELECT json_agg(row_to_json(p) ORDER BY p.created_at) FROM fleet_partner_communication_participants p WHERE p.organization_id=th.organization_id AND p.thread_id=th.id),'[]'::json) participants FROM fleet_partner_communication_threads th WHERE th.organization_id=$1 AND th.id=$2) x`, organizationID, id))
}

// ListThreadsForUser lists the threads a user participates in, within the
// permitted branches (empty means organization-wide).
func (r *Repository) ListThreadsForUser(ctx context.Context, organizationID, userID string, branchIDs []string) ([]map[string]any, error) {
	rows, err := r.pool.Query(ctx, `SELECT row_to_json(th) FROM fleet_partner_communication_threads th JOIN fleet_partner_communication_participants p ON p.thread_id=th.id AND p.organization_id=th.organization_id WHERE th.organization_id=$1 AND p.participant_type='USER' AND p.participant_reference=$2 AND (COALESCE(cardinality($3::text[]),0)=0 OR th.branch_id IS NULL OR th.branch_id=ANY($3)) ORDER BY th.updated_at DESC`, organizationID, userID, branchIDs)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}

// LockThread reads a thread's status and version FOR UPDATE.
func (r *Repository) LockThread(ctx context.Context, q database.Querier, organizationID, id string) (status string, version int64, err error) {
	err = r.q(q).QueryRow(ctx, `SELECT status,version FROM fleet_partner_communication_threads WHERE organization_id=$1 AND id=$2 FOR UPDATE`, organizationID, id).Scan(&status, &version)
	return status, version, err
}

// UpdateThread applies an edit and returns the new version.
func (r *Repository) UpdateThread(ctx context.Context, q database.Querier, organizationID, id, title, ownerUserID, status string) (int64, error) {
	var nv int64
	err := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_communication_threads SET title=CASE WHEN $3='' THEN title ELSE $3 END,owner_user_id=CASE WHEN $4='' THEN owner_user_id ELSE $4 END,status=$5,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, organizationID, id, title, ownerUserID, status).Scan(&nv)
	return nv, err
}

// CloseThread closes a thread and returns the new version.
func (r *Repository) CloseThread(ctx context.Context, q database.Querier, organizationID, id string) (int64, error) {
	var nv int64
	err := r.q(q).QueryRow(ctx, `UPDATE fleet_partner_communication_threads SET status='CLOSED',closed_at=now(),version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2 RETURNING version`, organizationID, id).Scan(&nv)
	return nv, err
}

// InsertThreadHistory appends a thread transition.
func (r *Repository) InsertThreadHistory(ctx context.Context, q database.Querier, organizationID, threadID, previousStatus, newStatus string, snapshot []byte, actorID string, version int64) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_communication_thread_history(id,organization_id,thread_id,previous_status,new_status,snapshot,actor_reference,version) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, ids.New("cth"), organizationID, threadID, previousStatus, newStatus, snapshot, actorID, version)
	return err
}

// LockParticipantThread locks a thread the user participates in and returns
// its status and visibility.
func (r *Repository) LockParticipantThread(ctx context.Context, q database.Querier, organizationID, threadID, userID string) (status, visibility string, err error) {
	var version int64
	err = r.q(q).QueryRow(ctx, `SELECT th.status,th.visibility,th.version FROM fleet_partner_communication_threads th JOIN fleet_partner_communication_participants p ON p.thread_id=th.id AND p.organization_id=th.organization_id WHERE th.organization_id=$1 AND th.id=$2 AND p.participant_type='USER' AND p.participant_reference=$3 FOR UPDATE OF th`, organizationID, threadID, userID).Scan(&status, &visibility, &version)
	return status, visibility, err
}

// ExternalRecipients lists the thread's customer and provider participants.
func (r *Repository) ExternalRecipients(ctx context.Context, q database.Querier, organizationID, threadID string) ([]models.Recipient, error) {
	rows, err := r.q(q).Query(ctx, `SELECT participant_type,participant_reference FROM fleet_partner_communication_participants WHERE organization_id=$1 AND thread_id=$2 AND participant_type IN('CUSTOMER','PROVIDER') ORDER BY participant_type,participant_reference`, organizationID, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Recipient
	for rows.Next() {
		var item models.Recipient
		if err := rows.Scan(&item.Type, &item.Reference); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// MarkThreadPendingResponse records that a message awaits a response.
func (r *Repository) MarkThreadPendingResponse(ctx context.Context, q database.Querier, organizationID, threadID string) error {
	_, err := r.q(q).Exec(ctx, `UPDATE fleet_partner_communication_threads SET status='PENDING_RESPONSE',version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, organizationID, threadID)
	return err
}

// ---------------------------------------------------------------------------
// Messages
// ---------------------------------------------------------------------------

// MessageByIdempotencyKey reads a message by its idempotency key.
func (r *Repository) MessageByIdempotencyKey(ctx context.Context, organizationID, key string) (map[string]any, error) {
	return jsonRow(r.pool.QueryRow(ctx, `SELECT row_to_json(m) FROM fleet_partner_communication_messages m WHERE organization_id=$1 AND idempotency_key=$2`, organizationID, key))
}

// GetMessage reads one message.
func (r *Repository) GetMessage(ctx context.Context, organizationID, id string) (map[string]any, error) {
	return jsonRow(r.pool.QueryRow(ctx, `SELECT row_to_json(m) FROM fleet_partner_communication_messages m WHERE organization_id=$1 AND id=$2`, organizationID, id))
}

// InsertMessage stores a message sent by a user.
func (r *Repository) InsertMessage(ctx context.Context, q database.Querier, id, organizationID, threadID, actorID string, in models.MessageInput, media []byte, deliveryStatus, key string) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_communication_messages(id,organization_id,thread_id,sender_type,sender_reference,channel,text,media_file_ids,delivery_status,reply_to_message_id,idempotency_key,source_timestamp) VALUES($1,$2,$3,'USER',$4,$5,$6,$7,$8,NULLIF($9,''),$10,$11)`, id, organizationID, threadID, actorID, in.Channel, in.Text, media, deliveryStatus, in.ReplyToMessageID, key, in.SourceTimestamp)
	return err
}

// ListMessages lists a thread's messages oldest first.
func (r *Repository) ListMessages(ctx context.Context, organizationID, threadID string) ([]map[string]any, error) {
	rows, err := r.pool.Query(ctx, `SELECT row_to_json(m) FROM fleet_partner_communication_messages m WHERE organization_id=$1 AND thread_id=$2 ORDER BY created_at`, organizationID, threadID)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}

// ---------------------------------------------------------------------------
// Notification rules
// ---------------------------------------------------------------------------

// LockRule reads a rule by event and recipient scope FOR UPDATE.
func (r *Repository) LockRule(ctx context.Context, q database.Querier, organizationID, eventType, recipientScope string) (id string, version int64, err error) {
	err = r.q(q).QueryRow(ctx, `SELECT id,version FROM fleet_partner_notification_rules WHERE organization_id=$1 AND event_type=$2 AND recipient_scope=$3 FOR UPDATE`, organizationID, eventType, recipientScope).Scan(&id, &version)
	return id, version, err
}

// UpdateRule rewrites a rule's delivery settings.
func (r *Repository) UpdateRule(ctx context.Context, q database.Querier, organizationID, id string, channels, quiet json.RawMessage, timezone, severity string, escalation json.RawMessage, enabled bool) error {
	_, err := r.q(q).Exec(ctx, `UPDATE fleet_partner_notification_rules SET channels=$3,quiet_hours=$4,timezone=$5,severity_threshold=$6,escalation_policy=$7,enabled=$8,version=version+1,updated_at=now() WHERE organization_id=$1 AND id=$2`, organizationID, id, channels, quiet, timezone, severity, escalation, enabled)
	return err
}

// InsertRule creates a rule.
func (r *Repository) InsertRule(ctx context.Context, q database.Querier, id, organizationID, eventType, recipientScope string, channels, quiet json.RawMessage, timezone, severity string, escalation json.RawMessage, enabled bool) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_notification_rules(id,organization_id,event_type,recipient_scope,channels,quiet_hours,timezone,severity_threshold,escalation_policy,enabled) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, id, organizationID, eventType, recipientScope, channels, quiet, timezone, severity, escalation, enabled)
	return err
}

// GetRule reads one rule.
func (r *Repository) GetRule(ctx context.Context, organizationID, id string) (map[string]any, error) {
	return jsonRow(r.pool.QueryRow(ctx, `SELECT row_to_json(r) FROM fleet_partner_notification_rules r WHERE organization_id=$1 AND id=$2`, organizationID, id))
}

// ListRules lists the organization's rules.
func (r *Repository) ListRules(ctx context.Context, organizationID string) ([]map[string]any, error) {
	rows, err := r.pool.Query(ctx, `SELECT row_to_json(r) FROM fleet_partner_notification_rules r WHERE organization_id=$1 ORDER BY event_type,recipient_scope`, organizationID)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}

// ---------------------------------------------------------------------------
// Notification intents and delivery attempts
// ---------------------------------------------------------------------------

// InsertIntent records a durable notification intent.
func (r *Repository) InsertIntent(ctx context.Context, q database.Querier, id string, in models.IntentInput) error {
	sql := `INSERT INTO fleet_partner_notification_intents(id,organization_id,source_type,source_reference,recipient_type,recipient_reference,channels,template_code,safe_payload,idempotency_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`
	if in.IgnoreDuplicate {
		sql += ` ON CONFLICT(organization_id,idempotency_key) DO NOTHING`
	}
	_, err := r.q(q).Exec(ctx, sql, id, in.OrganizationID, in.SourceType, in.SourceReference, in.RecipientType, in.RecipientReference, in.Channels, in.TemplateCode, in.SafePayload, in.IdempotencyKey)
	return err
}

// ActiveOrganizations lists the organizations the maintenance pass covers.
func (r *Repository) ActiveOrganizations(ctx context.Context) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT id FROM fleet_partner_organizations WHERE status='ACTIVE' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	return scanStrings(rows)
}

// DueIntents lists intents whose outcome must be submitted again or read back.
func (r *Repository) DueIntents(ctx context.Context, organizationID string) ([]string, error) {
	rows, err := r.pool.Query(ctx, `SELECT id FROM fleet_partner_notification_intents WHERE organization_id=$1 AND status IN('OUTCOME_UNKNOWN','FAILED') AND next_attempt_at<=now() ORDER BY next_attempt_at LIMIT 50`, organizationID)
	if err != nil {
		return nil, err
	}
	return scanStrings(rows)
}

// GetIntent reads an intent's delivery state.
func (r *Repository) GetIntent(ctx context.Context, organizationID, id string) (models.Intent, error) {
	var i models.Intent
	err := r.pool.QueryRow(ctx, `SELECT idempotency_key,status,recipient_type,recipient_reference,template_code,safe_payload,channels,attempts FROM fleet_partner_notification_intents WHERE organization_id=$1 AND id=$2`, organizationID, id).Scan(&i.IdempotencyKey, &i.Status, &i.RecipientType, &i.RecipientReference, &i.TemplateCode, &i.SafePayload, &i.Channels, &i.Attempts)
	return i, err
}

// RecordDeliveryAttempt appends an attempt and moves the intent to status.
func (r *Repository) RecordDeliveryAttempt(ctx context.Context, q database.Querier, organizationID, intentID string, attemptNumber int, outcome, reference, code string, started time.Time, status string) error {
	if _, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_notification_delivery_attempts(id,organization_id,intent_id,attempt_number,outcome,operation_reference,error_code,started_at,completed_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,now())`, ids.New("nda"), organizationID, intentID, attemptNumber, outcome, reference, code, started); e != nil {
		return e
	}
	_, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_notification_intents SET status=$3,delivery_operation_reference=$4,attempts=attempts+1,last_error_code=$5,next_attempt_at=CASE WHEN $3 IN('FAILED','OUTCOME_UNKNOWN') THEN now()+interval '30 seconds' ELSE next_attempt_at END,updated_at=now() WHERE organization_id=$1 AND id=$2`, organizationID, intentID, status, reference, code)
	return e
}

// UpsertPreference creates or, at the expected version, updates one
// preference. It returns the new version; a stale version returns 0.
func (r *Repository) UpsertPreference(ctx context.Context, q database.Querier, id, organizationID, subjectType, subjectID, eventType string, channels, quiet []byte, enabled bool, expected int64) (int64, error) {
	var version int64
	err := r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_notification_preferences(id,organization_id,subject_type,subject_id,event_type,channels,quiet_hours,enabled)VALUES($1,$2,$3,$4,$5,$6,$7,$8)
 ON CONFLICT(organization_id,subject_type,subject_id,event_type) DO UPDATE SET channels=excluded.channels,quiet_hours=excluded.quiet_hours,enabled=excluded.enabled,version=fleet_partner_notification_preferences.version+1,updated_at=now()
 WHERE fleet_partner_notification_preferences.version=$9 RETURNING version`, id, organizationID, subjectType, subjectID, eventType, channels, quiet, enabled, expected).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return version, err
}

// ListPreferences lists the organization defaults and the caller's own
// preferences.
func (r *Repository) ListPreferences(ctx context.Context, organizationID, userID string) ([]map[string]any, error) {
	rows, err := r.pool.Query(ctx, `SELECT row_to_json(p) FROM fleet_partner_notification_preferences p WHERE organization_id=$1 AND (subject_type='ORGANIZATION' OR (subject_type='USER' AND subject_id=$2)) ORDER BY subject_type,event_type`, organizationID, userID)
	if err != nil {
		return nil, err
	}
	return jsonList(rows)
}

// PreferenceFor resolves the effective preference for a user recipient and
// event: the user's own, else the organization default, else none.
func (r *Repository) PreferenceFor(ctx context.Context, q database.Querier, organizationID, userID, eventType string) (channels []byte, enabled bool, found bool, err error) {
	err = r.q(q).QueryRow(ctx, `SELECT channels,enabled FROM fleet_partner_notification_preferences WHERE organization_id=$1 AND event_type=$3 AND ((subject_type='USER' AND subject_id=$2) OR subject_type='ORGANIZATION') ORDER BY CASE subject_type WHEN 'USER' THEN 0 ELSE 1 END LIMIT 1`, organizationID, userID, eventType).Scan(&channels, &enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, false, nil
	}
	return channels, enabled, err == nil, err
}

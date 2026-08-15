package crm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/notifications"
)

const (
	CampaignRunScheduled             = "SCHEDULED"
	CampaignRunQueued                = "QUEUED"
	CampaignRunRunning               = "RUNNING"
	CampaignRunPaused                = "PAUSED"
	CampaignRunCompleted             = "COMPLETED"
	CampaignRunCompletedWithFailures = "COMPLETED_WITH_FAILURES"
	CampaignRunCancelled             = "CANCELLED"
	CampaignRunFailed                = "FAILED"

	DeliveryPending     = "PENDING"
	DeliveryDeferred    = "DEFERRED_QUIET_HOURS"
	DeliverySuppressed  = "SUPPRESSED"
	DeliveryHandedOff   = "HANDED_OFF"
	DeliveryAccepted    = "ACCEPTED"
	DeliveryDelivered   = "DELIVERED"
	DeliveryFailedRetry = "FAILED_TRANSIENT"
	DeliveryFailedFinal = "FAILED_PERMANENT"
	DeliveryCancelled   = "CANCELLED"

	CodeCampaignNotPrepared        = "CRM_CAMPAIGN_NOT_PREPARED"
	CodeCampaignRunNotFound        = "CRM_CAMPAIGN_RUN_NOT_FOUND"
	CodeCampaignRunInvalidState    = "CRM_CAMPAIGN_RUN_INVALID_STATE"
	CodeCampaignRunVersionConflict = "CRM_CAMPAIGN_RUN_VERSION_CONFLICT"
)

type CampaignRun struct {
	ID, BusinessID, CampaignID, CampaignVersionID, SnapshotID string
	CampaignDefinitionVersion                                 int64
	TriggerType                                               string
	TriggerRef                                                *string
	Mode, Status                                              string
	ScheduledFor                                              time.Time
	TemplateKey, Locale, Title, Body                          *string
	TotalDeliveries, HandedOffCount, DeliveredCount           int
	SuppressedCount, FailedCount                              int
	RequestedByStaffRef                                       *string
	CreatedAt, UpdatedAt                                      time.Time
	StartedAt, CompletedAt, CancelledAt                       *time.Time
	Version                                                   int64
}

type CampaignDelivery struct {
	ID, BusinessID, RunID, CampaignID, SnapshotID, TargetID, ProfileID string
	Channel, Status, NotificationRequestRef, AttributionRef            string
	VariantKey, SuppressionReason, NotificationMessageRef              *string
	ConsentDecisionAtHandoff                                           *string
	ConsentEligibleAtHandoff                                           *bool
	ConsentCheckedAt, ConsentAsOf, PolicyCheckedAt                     *time.Time
	ConsentSourceVersion                                               *int64
	IsControl                                                          bool
	NextAttemptAt                                                      time.Time
	AttemptCount                                                       int
	LastEventVersion, Version                                          int64
	CreatedAt, UpdatedAt                                               time.Time
	HandedOffAt, CompletedAt                                           *time.Time
}

type CampaignDeliveryHistory struct {
	ID            string    `json:"history_ref"`
	DeliveryID    string    `json:"delivery_ref"`
	ToStatus      string    `json:"to_status"`
	Source        string    `json:"source"`
	FromStatus    *string   `json:"from_status,omitempty"`
	ReasonCode    *string   `json:"reason_code,omitempty"`
	SourceEventID *string   `json:"source_event_ref,omitempty"`
	SourceVersion *int64    `json:"source_version,omitempty"`
	OccurredAt    time.Time `json:"occurred_at"`
	RecordedAt    time.Time `json:"recorded_at"`
}

type CreateCampaignRunInput struct {
	SnapshotRef                      string
	Mode                             string
	ScheduledFor                     *time.Time
	ExpectedVersion                  int64
	TemplateKey, Locale, Title, Body string
	TriggerType, TriggerRef          string
	ExactDefinitionVersion           int64
}

const runCols = `id,business_id,campaign_id,campaign_version_id,campaign_definition_version,snapshot_id,
trigger_type,trigger_ref,mode,status,scheduled_for,template_key,locale,title,body,total_deliveries,
handed_off_count,delivered_count,suppressed_count,failed_count,requested_by_staff_ref,created_at,
updated_at,started_at,completed_at,cancelled_at,version`

func scanCampaignRun(row rowScanner) (CampaignRun, error) {
	var r CampaignRun
	err := row.Scan(&r.ID, &r.BusinessID, &r.CampaignID, &r.CampaignVersionID, &r.CampaignDefinitionVersion,
		&r.SnapshotID, &r.TriggerType, &r.TriggerRef, &r.Mode, &r.Status, &r.ScheduledFor,
		&r.TemplateKey, &r.Locale, &r.Title, &r.Body, &r.TotalDeliveries, &r.HandedOffCount,
		&r.DeliveredCount, &r.SuppressedCount, &r.FailedCount, &r.RequestedByStaffRef,
		&r.CreatedAt, &r.UpdatedAt, &r.StartedAt, &r.CompletedAt, &r.CancelledAt, &r.Version)
	return r, err
}

const deliveryCols = `id,business_id,run_id,campaign_id,snapshot_id,target_id,customer_profile_id,
channel,variant_key,is_control,status,suppression_reason,notification_request_ref,
notification_message_ref,attribution_ref,consent_decision_at_handoff,consent_eligible_at_handoff,
consent_checked_at,consent_as_of,consent_source_version,policy_checked_at,next_attempt_at,attempt_count,last_event_version,
created_at,updated_at,handed_off_at,completed_at,version`

func scanCampaignDelivery(row rowScanner) (CampaignDelivery, error) {
	var d CampaignDelivery
	err := row.Scan(&d.ID, &d.BusinessID, &d.RunID, &d.CampaignID, &d.SnapshotID, &d.TargetID,
		&d.ProfileID, &d.Channel, &d.VariantKey, &d.IsControl, &d.Status, &d.SuppressionReason,
		&d.NotificationRequestRef, &d.NotificationMessageRef, &d.AttributionRef,
		&d.ConsentDecisionAtHandoff, &d.ConsentEligibleAtHandoff, &d.ConsentCheckedAt,
		&d.ConsentAsOf, &d.ConsentSourceVersion, &d.PolicyCheckedAt, &d.NextAttemptAt,
		&d.AttemptCount, &d.LastEventVersion, &d.CreatedAt, &d.UpdatedAt, &d.HandedOffAt,
		&d.CompletedAt, &d.Version)
	return d, err
}

func (s *Service) assertCampaignExecutionWrites() error {
	if err := s.assertCampaignWrites(); err != nil {
		return err
	}
	if !s.flags.CampaignExecutionEnabled {
		return apperrors.Unavailable("Campaign execution is temporarily disabled.", nil)
	}
	return nil
}

func validateRunInput(in *CreateCampaignRunInput, now time.Time) error {
	in.SnapshotRef = strings.TrimSpace(in.SnapshotRef)
	in.Mode = strings.ToUpper(strings.TrimSpace(in.Mode))
	in.TemplateKey, in.Locale = strings.TrimSpace(in.TemplateKey), strings.TrimSpace(in.Locale)
	in.Title, in.Body = strings.TrimSpace(in.Title), strings.TrimSpace(in.Body)
	if in.SnapshotRef == "" {
		return campaignValidation("snapshot_ref", "A prepared audience snapshot is required.", CodeCampaignNotPrepared)
	}
	if in.Mode != "SEND_NOW" && in.Mode != "SCHEDULED" {
		return campaignValidation("mode", "Mode must be SEND_NOW or SCHEDULED.", CodeCampaignRunInvalidState)
	}
	if in.Mode == "SCHEDULED" && (in.ScheduledFor == nil || !in.ScheduledFor.After(now)) {
		return campaignValidation("scheduled_for", "A scheduled run requires a future RFC3339 time.", CodeCampaignRunInvalidState)
	}
	if in.ExpectedVersion <= 0 {
		return campaignValidation("expected_version", "Expected version is required.", CodeCampaignRunVersionConflict)
	}
	if in.TemplateKey == "" && in.Title == "" && in.Body == "" {
		return campaignValidation("template_key", "Provide a template key or bounded plain-text title/body.", CodeCampaignRunInvalidState)
	}
	if len(in.TemplateKey) > 120 || len(in.Locale) > 16 || len(in.Title) > 200 || len(in.Body) > 4000 {
		return campaignValidation("content", "Campaign content exceeds the allowed length.", CodeCampaignRunInvalidState)
	}
	if strings.ContainsAny(in.Title+in.Body, "<>") {
		return campaignValidation("content", "Campaign content must be plain text.", CodeCampaignRunInvalidState)
	}
	return nil
}

// CreateCampaignRun freezes the exact prepared snapshot/version into a durable
// run and materializes one delivery per snapshot-eligible channel. It does not
// contact Notification Service; workers re-check every policy at handoff time.
func (s *Service) CreateCampaignRun(ctx context.Context, actor Actor, scope Scope, campaignID string, in CreateCampaignRunInput) (CampaignRun, error) {
	if err := s.assertCampaignExecutionWrites(); err != nil {
		return CampaignRun{}, err
	}
	now := s.now()
	if err := validateRunInput(&in, now); err != nil {
		return CampaignRun{}, err
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return CampaignRun{}, err
	}
	defer tx.Rollback(ctx)
	campaign, err := s.loadCampaignForUpdate(ctx, tx, actor.VendorID, campaignID)
	if err != nil {
		return CampaignRun{}, err
	}
	if err = s.assertCampaignVisible(scope, campaign); err != nil {
		return CampaignRun{}, err
	}
	if campaign.Version != in.ExpectedVersion {
		return CampaignRun{}, crmError(apperrors.Conflict("Campaign changed since you loaded it.", nil), CodeCampaignRunVersionConflict)
	}
	if in.ExactDefinitionVersion > 0 && campaign.CurrentDefinitionVersion != in.ExactDefinitionVersion {
		return CampaignRun{}, crmError(apperrors.Conflict("Campaign definition no longer matches the trigger.", nil), CodeCampaignRunVersionConflict)
	}
	if campaign.Status != CampaignPrepared || campaign.CurrentSnapshotID == nil || *campaign.CurrentSnapshotID != in.SnapshotRef {
		return CampaignRun{}, crmError(apperrors.Conflict("Campaign must use its current prepared audience.", nil), CodeCampaignNotPrepared)
	}
	var snapStatus, versionID string
	var definitionVersion int64
	err = tx.QueryRow(ctx, `SELECT status,campaign_version_id,campaign_definition_version FROM crm_campaign_audience_snapshots
		WHERE business_id=$1 AND campaign_id=$2 AND id=$3 FOR SHARE`, actor.VendorID, campaignID, in.SnapshotRef).
		Scan(&snapStatus, &versionID, &definitionVersion)
	if errors.Is(err, pgx.ErrNoRows) || snapStatus != "CURRENT" {
		return CampaignRun{}, crmError(apperrors.Conflict("Audience snapshot is not current.", nil), CodeCampaignNotPrepared)
	}
	if err != nil {
		return CampaignRun{}, err
	}
	scheduled := now
	status := CampaignRunQueued
	if in.Mode == "SCHEDULED" {
		scheduled = in.ScheduledFor.UTC()
		status = CampaignRunScheduled
	}
	triggerType := strings.ToUpper(strings.TrimSpace(in.TriggerType))
	if triggerType == "" {
		triggerType = "MANUAL"
	}
	if triggerType != "MANUAL" && triggerType != "EVENT" {
		return CampaignRun{}, campaignValidation("trigger_type", "Unsupported trigger type.", CodeCampaignRunInvalidState)
	}
	runID := "crmrun_" + uuid.NewString()
	staff := nullString(actor.StaffID)
	triggerRef := nullString(in.TriggerRef)
	_, err = tx.Exec(ctx, `INSERT INTO crm_campaign_runs
		(id,business_id,campaign_id,campaign_version_id,campaign_definition_version,snapshot_id,trigger_type,trigger_ref,
		 mode,status,scheduled_for,template_key,locale,title,body,requested_by_staff_ref,created_at,updated_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$17)`, runID, actor.VendorID,
		campaignID, versionID, definitionVersion, in.SnapshotRef, triggerType, triggerRef, in.Mode, status, scheduled,
		nullString(in.TemplateKey), nullString(in.Locale), nullString(in.Title), nullString(in.Body), staff, now)
	if err != nil {
		return CampaignRun{}, err
	}
	rows, err := tx.Query(ctx, `SELECT t.id,t.customer_profile_id,t.experiment_variant_key,t.is_control,ch.channel
		FROM crm_campaign_targets t JOIN crm_campaign_target_channels ch
		 ON ch.business_id=t.business_id AND ch.target_id=t.id
		WHERE t.business_id=$1 AND t.snapshot_id=$2 AND ch.snapshot_eligibility_state='ELIGIBLE_AT_SNAPSHOT'
		ORDER BY t.id,ch.channel`, actor.VendorID, in.SnapshotRef)
	if err != nil {
		return CampaignRun{}, err
	}
	type deliverySeed struct {
		targetID, profileID, channel string
		variant                      *string
		control                      bool
	}
	seeds := []deliverySeed{}
	for rows.Next() {
		var seed deliverySeed
		if err = rows.Scan(&seed.targetID, &seed.profileID, &seed.variant, &seed.control, &seed.channel); err != nil {
			rows.Close()
			return CampaignRun{}, err
		}
		seeds = append(seeds, seed)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return CampaignRun{}, err
	}
	rows.Close()
	total, suppressed := 0, 0
	for _, seed := range seeds {
		deliveryID := "crmdl_" + uuid.NewString()
		deliveryStatus, reason := DeliveryPending, any(nil)
		if seed.control {
			deliveryStatus, reason, suppressed = DeliverySuppressed, "CONTROL_GROUP", suppressed+1
		}
		requestRef := "vendor-service:crm.campaign.delivery:" + deliveryID
		attributionRef := "crmatr_" + uuid.NewString()
		_, err = tx.Exec(ctx, `INSERT INTO crm_campaign_deliveries
			(id,business_id,run_id,campaign_id,snapshot_id,target_id,customer_profile_id,channel,variant_key,is_control,
			 status,suppression_reason,notification_request_ref,attribution_ref,next_attempt_at,completed_at)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15::timestamptz,CASE WHEN $11::text='SUPPRESSED' THEN $15::timestamptz END)`,
			deliveryID, actor.VendorID, runID, campaignID, in.SnapshotRef, seed.targetID, seed.profileID, seed.channel, seed.variant,
			seed.control, deliveryStatus, reason, requestRef, attributionRef, scheduled)
		if err != nil {
			return CampaignRun{}, err
		}
		if err = s.appendDeliveryHistory(ctx, tx, actor.VendorID, deliveryID, nil, deliveryStatus, reasonString(reason), "VENDOR", "", 0, now); err != nil {
			return CampaignRun{}, err
		}
		total++
	}
	_, err = tx.Exec(ctx, `UPDATE crm_campaign_runs SET total_deliveries=$2,suppressed_count=$3 WHERE id=$1`, runID, total, suppressed)
	if err != nil {
		return CampaignRun{}, err
	}
	if err = s.auditTx(ctx, tx, actor, "crm.campaign.run.created", "crm_campaign_run", runID, derefOr(campaign.BranchID), nil,
		map[string]any{"campaign_ref": campaignID, "snapshot_ref": in.SnapshotRef, "mode": in.Mode, "delivery_count": total}, ""); err != nil {
		return CampaignRun{}, err
	}
	if err = s.publishTx(ctx, tx, actor, "vendor.crm.campaign.run.created", "crm_campaign_run", runID, 1,
		map[string]any{"campaign_ref": campaignID, "snapshot_ref": in.SnapshotRef, "scheduled_for": scheduled}); err != nil {
		return CampaignRun{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CampaignRun{}, err
	}
	s.metrics.Inc("crm_campaign_run_total", "mode", strings.ToLower(in.Mode))
	return s.loadRun(ctx, actor.VendorID, campaignID, runID)
}

func nullString(v string) any {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return strings.TrimSpace(v)
}
func reasonString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func (s *Service) loadRun(ctx context.Context, businessID, campaignID, runID string) (CampaignRun, error) {
	r, err := scanCampaignRun(s.db.QueryRow(ctx, `SELECT `+runCols+` FROM crm_campaign_runs WHERE business_id=$1 AND campaign_id=$2 AND id=$3`, businessID, campaignID, runID))
	if errors.Is(err, pgx.ErrNoRows) {
		return CampaignRun{}, crmError(apperrors.NotFound("Campaign run not found.", nil), CodeCampaignRunNotFound)
	}
	return r, err
}

func (s *Service) GetCampaignRun(ctx context.Context, actor Actor, scope Scope, campaignID, runID string) (CampaignRun, error) {
	if _, err := s.GetCampaign(ctx, actor, scope, campaignID); err != nil {
		return CampaignRun{}, err
	}
	return s.loadRun(ctx, actor.VendorID, campaignID, runID)
}

func (s *Service) ListCampaignRuns(ctx context.Context, actor Actor, scope Scope, campaignID string, limit, offset int) ([]CampaignRun, bool, error) {
	if _, err := s.GetCampaign(ctx, actor, scope, campaignID); err != nil {
		return nil, false, err
	}
	limit, offset = boundPage(limit, offset)
	rows, err := s.db.Query(ctx, `SELECT `+runCols+` FROM crm_campaign_runs WHERE business_id=$1 AND campaign_id=$2 ORDER BY created_at DESC,id LIMIT $3 OFFSET $4`, actor.VendorID, campaignID, limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := make([]CampaignRun, 0, limit)
	for rows.Next() {
		r, e := scanCampaignRun(rows)
		if e != nil {
			return nil, false, e
		}
		out = append(out, r)
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, rows.Err()
}

func (s *Service) mutateRun(ctx context.Context, actor Actor, scope Scope, campaignID, runID, action string, expectedVersion int64) (CampaignRun, error) {
	if err := s.assertCampaignExecutionWrites(); err != nil {
		return CampaignRun{}, err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return CampaignRun{}, err
	}
	defer tx.Rollback(ctx)
	campaign, err := s.loadCampaignForUpdate(ctx, tx, actor.VendorID, campaignID)
	if err != nil {
		return CampaignRun{}, err
	}
	if err = s.assertCampaignVisible(scope, campaign); err != nil {
		return CampaignRun{}, err
	}
	run, err := scanCampaignRun(tx.QueryRow(ctx, `SELECT `+runCols+` FROM crm_campaign_runs WHERE business_id=$1 AND campaign_id=$2 AND id=$3 FOR UPDATE`, actor.VendorID, campaignID, runID))
	if errors.Is(err, pgx.ErrNoRows) {
		return CampaignRun{}, crmError(apperrors.NotFound("Campaign run not found.", nil), CodeCampaignRunNotFound)
	}
	if err != nil {
		return CampaignRun{}, err
	}
	if expectedVersion <= 0 || run.Version != expectedVersion {
		return CampaignRun{}, crmError(apperrors.Conflict("Campaign run changed since you loaded it.", nil), CodeCampaignRunVersionConflict)
	}
	from, to := run.Status, ""
	switch action {
	case "pause":
		if from != CampaignRunRunning && from != CampaignRunQueued && from != CampaignRunScheduled {
			return CampaignRun{}, invalidRunState()
		}
		to = CampaignRunPaused
	case "resume":
		if from != CampaignRunPaused {
			return CampaignRun{}, invalidRunState()
		}
		if run.ScheduledFor.After(s.now()) {
			to = CampaignRunScheduled
		} else {
			to = CampaignRunQueued
		}
	case "cancel":
		if from == CampaignRunCompleted || from == CampaignRunCompletedWithFailures || from == CampaignRunCancelled || from == CampaignRunFailed {
			return CampaignRun{}, invalidRunState()
		}
		to = CampaignRunCancelled
	default:
		return CampaignRun{}, invalidRunState()
	}
	now := s.now()
	type cancelledDelivery struct{ id, status string }
	var cancelled []cancelledDelivery
	if to == CampaignRunCancelled {
		rows, queryErr := tx.Query(ctx, `SELECT id,status FROM crm_campaign_deliveries WHERE run_id=$1 AND status IN ('PENDING','DEFERRED_QUIET_HOURS','FAILED_TRANSIENT') FOR UPDATE`, runID)
		if queryErr != nil {
			return CampaignRun{}, queryErr
		}
		for rows.Next() {
			var item cancelledDelivery
			if queryErr = rows.Scan(&item.id, &item.status); queryErr != nil {
				rows.Close()
				return CampaignRun{}, queryErr
			}
			cancelled = append(cancelled, item)
		}
		if queryErr = rows.Err(); queryErr != nil {
			rows.Close()
			return CampaignRun{}, queryErr
		}
		rows.Close()
	}
	_, err = tx.Exec(ctx, `UPDATE crm_campaign_runs SET status=$2,updated_at=$3,version=version+1,cancelled_at=CASE WHEN $2='CANCELLED' THEN $3 ELSE cancelled_at END WHERE id=$1`, runID, to, now)
	if err != nil {
		return CampaignRun{}, err
	}
	if to == CampaignRunCancelled {
		_, err = tx.Exec(ctx, `UPDATE crm_campaign_deliveries SET status='CANCELLED',updated_at=$2,completed_at=$2,version=version+1 WHERE run_id=$1 AND status IN ('PENDING','DEFERRED_QUIET_HOURS','FAILED_TRANSIENT')`, runID, now)
		if err != nil {
			return CampaignRun{}, err
		}
		for _, item := range cancelled {
			fromStatus := item.status
			if err = s.appendDeliveryHistory(ctx, tx, actor.VendorID, item.id, &fromStatus, DeliveryCancelled, "RUN_CANCELLED", "VENDOR", "", 0, now); err != nil {
				return CampaignRun{}, err
			}
		}
	}
	auditAction := map[string]string{"pause": "crm.campaign.run.paused", "resume": "crm.campaign.run.resumed", "cancel": "crm.campaign.run.cancelled"}[action]
	if err = s.auditTx(ctx, tx, actor, auditAction, "crm_campaign_run", runID, derefOr(campaign.BranchID), map[string]any{"status": from}, map[string]any{"status": to}, ""); err != nil {
		return CampaignRun{}, err
	}
	event := "vendor.crm.campaign.run." + action + "d"
	if action == "cancel" {
		event = "vendor.crm.campaign.run.cancelled"
	}
	if err = s.publishTx(ctx, tx, actor, event, "crm_campaign_run", runID, run.Version+1, map[string]any{"campaign_ref": campaignID, "status": to}); err != nil {
		return CampaignRun{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CampaignRun{}, err
	}
	return s.loadRun(ctx, actor.VendorID, campaignID, runID)
}

func invalidRunState() error {
	return crmError(apperrors.Conflict("Campaign run is not in a state that permits this action.", nil), CodeCampaignRunInvalidState)
}
func (s *Service) PauseCampaignRun(ctx context.Context, a Actor, sc Scope, c, r string, v int64) (CampaignRun, error) {
	return s.mutateRun(ctx, a, sc, c, r, "pause", v)
}
func (s *Service) ResumeCampaignRun(ctx context.Context, a Actor, sc Scope, c, r string, v int64) (CampaignRun, error) {
	return s.mutateRun(ctx, a, sc, c, r, "resume", v)
}
func (s *Service) CancelCampaignRun(ctx context.Context, a Actor, sc Scope, c, r string, v int64) (CampaignRun, error) {
	return s.mutateRun(ctx, a, sc, c, r, "cancel", v)
}

// RunCampaignSchedulerPass atomically promotes due runs. SKIP LOCKED permits
// many instances without duplicate starts.
func (s *Service) RunCampaignSchedulerPass(ctx context.Context, limit int) (int, error) {
	if !s.flags.CRMEnabled || !s.flags.CampaignsEnabled || !s.flags.CampaignExecutionEnabled || s.flags.CommerceKillSwitch {
		return 0, nil
	}
	if limit <= 0 {
		limit = 10
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT id,business_id FROM crm_campaign_runs WHERE status IN ('SCHEDULED','QUEUED') AND scheduled_for<=$2 ORDER BY scheduled_for,id FOR UPDATE SKIP LOCKED LIMIT $1`, limit, s.now())
	if err != nil {
		return 0, err
	}
	type key struct{ id, business string }
	var keys []key
	for rows.Next() {
		var k key
		if err = rows.Scan(&k.id, &k.business); err != nil {
			rows.Close()
			return 0, err
		}
		keys = append(keys, k)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, err
	}
	for _, k := range keys {
		now := s.now()
		_, err = tx.Exec(ctx, `UPDATE crm_campaign_runs SET status='RUNNING',started_at=COALESCE(started_at,$2),updated_at=$2,version=version+1 WHERE id=$1`, k.id, now)
		if err != nil {
			return 0, err
		}
		actor := Actor{VendorID: k.business}
		if err = s.auditTx(ctx, tx, actor, "crm.campaign.run.started", "crm_campaign_run", k.id, "", nil, map[string]any{"status": "RUNNING"}, ""); err != nil {
			return 0, err
		}
		if err = s.publishTx(ctx, tx, actor, "vendor.crm.campaign.run.started", "crm_campaign_run", k.id, 2, map[string]any{"status": "RUNNING"}); err != nil {
			return 0, err
		}
		if err = s.refreshRun(ctx, tx, k.id, now); err != nil {
			return 0, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(keys), nil
}

func nextQuietEnd(policy QuietHoursPolicy, at time.Time) (time.Time, error) {
	if !policy.Enabled {
		return at, nil
	}
	loc, err := time.LoadLocation(policy.Timezone)
	if err != nil {
		return time.Time{}, err
	}
	local := at.In(loc)
	blocked, err := policy.Blocks(at)
	if err != nil || !blocked {
		return at, err
	}
	y, m, d := local.Date()
	end := time.Date(y, m, d, policy.EndMinute/60, policy.EndMinute%60, 0, 0, loc)
	if policy.StartMinute > policy.EndMinute && local.Hour()*60+local.Minute() >= policy.StartMinute {
		end = end.AddDate(0, 0, 1)
	}
	if !end.After(local) {
		end = end.AddDate(0, 0, 1)
	}
	return end.UTC(), nil
}

// RunCampaignDeliveryPass performs one bounded set of policy-fenced handoffs.
// A fixed notification_request_ref and Notification Service idempotency make a
// timeout retry the same logical message, never a second message.
func (s *Service) RunCampaignDeliveryPass(ctx context.Context, limit int) (int, error) {
	if !s.flags.CRMEnabled || !s.flags.CampaignsEnabled || !s.flags.CampaignExecutionEnabled || s.flags.CommerceKillSwitch {
		return 0, nil
	}
	if limit <= 0 {
		limit = 25
	}
	processed := 0
	for processed < limit {
		ok, err := s.processOneDelivery(ctx)
		if err != nil {
			return processed, err
		}
		if !ok {
			break
		}
		processed++
	}
	return processed, nil
}

func (s *Service) processOneDelivery(ctx context.Context) (bool, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	d, err := scanCampaignDelivery(tx.QueryRow(ctx, `SELECT `+deliveryCols+` FROM crm_campaign_deliveries d
		WHERE d.status IN ('PENDING','DEFERRED_QUIET_HOURS','FAILED_TRANSIENT') AND d.next_attempt_at<=$1
		AND EXISTS(SELECT 1 FROM crm_campaign_runs r WHERE r.id=d.run_id AND r.business_id=d.business_id AND r.status='RUNNING')
		ORDER BY d.next_attempt_at,d.id FOR UPDATE SKIP LOCKED LIMIT 1`, s.now()))
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	run, err := scanCampaignRun(tx.QueryRow(ctx, `SELECT `+runCols+` FROM crm_campaign_runs WHERE id=$1 AND business_id=$2 FOR UPDATE`, d.RunID, d.BusinessID))
	if err != nil {
		return false, err
	}
	if run.Status != CampaignRunRunning {
		return false, nil
	}
	var customerRef *string
	var privacy, merge, status string
	var recovery, marketingSuppressed bool
	err = tx.QueryRow(ctx, `SELECT p.customer_ref,p.privacy_state,p.merge_state,p.status,`+serviceRecoverySQL+`,
		EXISTS(SELECT 1 FROM crm_privacy_projections pp WHERE pp.business_id=p.business_id AND pp.profile_id=p.id AND pp.action='MARKETING_SUPPRESS' AND pp.processing_status='APPLIED')
		FROM vendor_customer_profiles p WHERE p.business_id=$1 AND p.id=$2 FOR UPDATE`, d.BusinessID, d.ProfileID).Scan(&customerRef, &privacy, &merge, &status, &recovery, &marketingSuppressed)
	if err != nil {
		return false, err
	}
	now := s.now()
	suppress := func(reason string) (bool, error) {
		if err := s.transitionDelivery(ctx, tx, &d, DeliverySuppressed, reason, "VENDOR", "", 0, now); err != nil {
			return false, fmt.Errorf("suppress transition: %w", err)
		}
		if err := s.refreshRun(ctx, tx, run.ID, now); err != nil {
			return false, fmt.Errorf("suppress refresh run: %w", err)
		}
		return true, tx.Commit(ctx)
	}
	if privacy != string(PrivacyActive) || merge == string(MergeSource) || status != string(StatusActive) || marketingSuppressed || recovery || customerRef == nil || strings.TrimSpace(*customerRef) == "" {
		reason := "PRIVACY_OR_IDENTITY_INELIGIBLE"
		if recovery {
			reason = "SERVICE_RECOVERY"
		}
		return suppress(reason)
	}
	if d.Channel == "SMS" {
		return suppress("CHANNEL_UNAVAILABLE")
	}
	if s.notifier == nil {
		return suppress("NOTIFICATION_UNAVAILABLE")
	}
	if !s.flags.ConsentIntegrationEnabled {
		if err = s.recordDeliveryConsentCheck(ctx, tx, d.ID, "UNKNOWN", false, nil, nil, now); err != nil {
			return false, err
		}
		return suppress("CONSENT_UNAVAILABLE")
	}
	var decision string
	var observed time.Time
	var expires *time.Time
	var consentSourceVersion int64
	err = tx.QueryRow(ctx, `SELECT decision,observed_at,expires_at,source_version FROM crm_consent_projections WHERE business_id=$1 AND profile_id=$2 AND purpose='MARKETING' AND channel=$3`, d.BusinessID, d.ProfileID, d.Channel).Scan(&decision, &observed, &expires, &consentSourceVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		if err = s.recordDeliveryConsentCheck(ctx, tx, d.ID, "UNKNOWN", false, nil, nil, now); err != nil {
			return false, err
		}
		return suppress("CONSENT_UNKNOWN")
	}
	if err != nil {
		return false, err
	}
	eligible := decision == "ALLOW" && now.Sub(observed) <= ConsentStaleAfter && (expires == nil || expires.After(now))
	if err = s.recordDeliveryConsentCheck(ctx, tx, d.ID, decision, eligible, &observed, &consentSourceVersion, now); err != nil {
		return false, err
	}
	if !eligible {
		return suppress("CONSENT_NOT_ALLOWED")
	}
	version, err := s.CampaignVersionAt(ctx, tx, d.BusinessID, d.CampaignID, run.CampaignDefinitionVersion)
	if err != nil {
		return false, err
	}
	blocked, err := version.QuietHours.Blocks(now)
	if err != nil {
		return false, err
	}
	if blocked {
		next, err := nextQuietEnd(version.QuietHours, now)
		if err != nil {
			return false, err
		}
		from := d.Status
		_, err = tx.Exec(ctx, `UPDATE crm_campaign_deliveries SET status='DEFERRED_QUIET_HOURS',next_attempt_at=$2,updated_at=$3,version=version+1 WHERE id=$1`, d.ID, next, now)
		if err != nil {
			return false, err
		}
		if err = s.appendDeliveryHistory(ctx, tx, d.BusinessID, d.ID, &from, DeliveryDeferred, "QUIET_HOURS", "VENDOR", "", 0, now); err != nil {
			return false, err
		}
		return true, tx.Commit(ctx)
	}
	if version.Frequency.Enabled {
		var count int
		since := now.Add(-time.Duration(version.Frequency.WindowMinutes) * time.Minute)
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM crm_campaign_contact_history WHERE business_id=$1 AND customer_profile_id=$2 AND contacted_at>=$3`, d.BusinessID, d.ProfileID, since).Scan(&count); err != nil {
			return false, err
		}
		if count >= version.Frequency.Cap {
			return suppress("FREQUENCY_CAP")
		}
	}
	channel := map[string]string{"EMAIL": notifications.ChannelEmail, "PUSH": notifications.ChannelPush, "IN_APP": notifications.ChannelInApp}[d.Channel]
	req := notifications.Request{IDempotencyKey: d.NotificationRequestRef, SourceService: "vendor-service", EventType: "vendor.crm.campaign.delivery", Recipient: notifications.Recipient{Type: notifications.RecipientCustomer, ID: *customerRef}, Channels: []string{channel}, Priority: notifications.PriorityNormal, Data: map[string]any{"campaign_ref": d.CampaignID, "run_ref": d.RunID, "delivery_ref": d.ID, "attribution_ref": d.AttributionRef}}
	if run.TemplateKey != nil {
		req.TemplateKey = *run.TemplateKey
	}
	if run.Locale != nil {
		req.Locale = *run.Locale
	}
	if run.Title != nil {
		req.Title = *run.Title
	}
	if run.Body != nil {
		req.Body = *run.Body
	}

	// Reserve the delivery and COMMIT before the network call.
	//
	// Everything above ran inside a transaction holding three row locks: the
	// delivery, the run, and — critically — the customer profile, which is the
	// same fence the privacy and identity-merge handlers take. Calling
	// notification-service while holding that lock means a slow or hanging
	// remote service blocks redaction for that customer, and pins a database
	// connection for the whole round trip. Section 8I moves the handoff out.
	//
	// The reservation needs no new state: pushing next_attempt_at beyond the
	// handoff window makes the claim query skip this row, so no second worker
	// adopts it while the call is in flight. If this process dies before the
	// result is recorded, the row simply becomes claimable again when the
	// window lapses, and the retry calls notification-service with the SAME
	// NotificationRequestRef — so the remote side deduplicates and the customer
	// is not contacted twice. That stable identity is what makes splitting the
	// transaction safe rather than merely faster.
	reservedUntil := now.Add(s.campaignEval.HandoffReservation)
	if _, err = tx.Exec(ctx, `UPDATE crm_campaign_deliveries
		SET attempt_count=attempt_count+1, next_attempt_at=$2, updated_at=$3, version=version+1
		WHERE id=$1`, d.ID, reservedUntil, now); err != nil {
		return false, err
	}
	// Contact history is recorded HERE, under the profile lock, rather than
	// after the handoff returns. That placement is load-bearing: the frequency
	// cap counts these rows, so writing them in a later transaction would let
	// two concurrent deliveries for the same customer both pass the cap check
	// before either recorded a contact. Committing the count under the same
	// lock that gated the check is what keeps the fence exact.
	//
	// The cost is that a delivery whose handoff then fails still consumes one
	// slot in the window. That is the conservative direction, and the cap
	// exists to protect the customer from being contacted too often rather
	// than to guarantee a vendor uses every slot.
	if _, err = tx.Exec(ctx, `INSERT INTO crm_campaign_contact_history(id,business_id,customer_profile_id,campaign_id,run_id,delivery_id,channel,contacted_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(business_id,delivery_id) DO NOTHING`, "crmcon_"+uuid.NewString(), d.BusinessID, d.ProfileID, d.CampaignID, d.RunID, d.ID, d.Channel, now); err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}

	resp, sendErr := s.notifier.Send(ctx, req)

	// Persist the outcome in its own transaction, guarded so a cancellation
	// that committed while the call was in flight wins: a delivery that is no
	// longer in a sendable state is left exactly as the cancellation left it.
	return s.recordHandoffResult(ctx, d, run, resp, sendErr)
}

// recordHandoffResult writes the outcome of one notification handoff.
//
// The WHERE clause is the fence. A run cancelled or a delivery suppressed while
// the network call was in flight has already moved the row out of a sendable
// status, and this update then matches nothing — so the outcome is discarded
// rather than resurrecting work an operator stopped. The remote side keeps its
// own idempotency record either way, which is why discarding is safe.
func (s *Service) recordHandoffResult(ctx context.Context, d CampaignDelivery, run CampaignRun,
	resp notifications.SendResponse, sendErr error) (bool, error) {
	now := s.now()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	current, err := scanCampaignDelivery(tx.QueryRow(ctx, `SELECT `+deliveryCols+`
		FROM crm_campaign_deliveries d WHERE d.id=$1 FOR UPDATE`, d.ID))
	if err != nil {
		return false, err
	}
	switch current.Status {
	case DeliveryPending, DeliveryDeferred, DeliveryFailedRetry:
	default:
		// Cancelled, suppressed or already resolved while we were away.
		return true, tx.Commit(ctx)
	}
	from := current.Status

	if sendErr != nil {
		to := DeliveryFailedRetry
		if current.AttemptCount >= 5 {
			to = DeliveryFailedFinal
		}
		delay := time.Duration(1<<minInt(current.AttemptCount, 6)) * time.Minute
		if _, err = tx.Exec(ctx, `UPDATE crm_campaign_deliveries
			SET status=$2::text, next_attempt_at=$3::timestamptz, updated_at=$4::timestamptz,
			    completed_at=CASE WHEN $2::text='FAILED_PERMANENT' THEN $4::timestamptz END,
			    version=version+1 WHERE id=$1`, d.ID, to, now.Add(delay), now); err != nil {
			return false, err
		}
		if err = s.appendDeliveryHistory(ctx, tx, d.BusinessID, d.ID, &from, to,
			"NOTIFICATION_HANDOFF_FAILED", "VENDOR", "", 0, now); err != nil {
			return false, err
		}
		if err = s.refreshRun(ctx, tx, run.ID, now); err != nil {
			return false, err
		}
		return true, tx.Commit(ctx)
	}

	if _, err = tx.Exec(ctx, `UPDATE crm_campaign_deliveries
		SET status='HANDED_OFF', notification_message_ref=$2, handed_off_at=$3, updated_at=$3,
		    version=version+1 WHERE id=$1`, d.ID, resp.MessageID, now); err != nil {
		return false, err
	}
	if err = s.appendDeliveryHistory(ctx, tx, d.BusinessID, d.ID, &from, DeliveryHandedOff, "", "VENDOR", "", 0, now); err != nil {
		return false, err
	}
	if err = s.refreshRun(ctx, tx, run.ID, now); err != nil {
		return false, err
	}
	actor := Actor{VendorID: d.BusinessID}
	if err = s.auditTx(ctx, tx, actor, "crm.campaign.delivery.handed_off", "crm_campaign_delivery", d.ID, "", nil, map[string]any{"run_ref": d.RunID, "campaign_ref": d.CampaignID, "channel": d.Channel}, ""); err != nil {
		return false, err
	}
	if err = s.publishTx(ctx, tx, actor, "vendor.crm.campaign.delivery.handed_off", "crm_campaign_delivery", d.ID, current.Version+1, map[string]any{"campaign_ref": d.CampaignID, "run_ref": d.RunID, "channel": d.Channel}); err != nil {
		return false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	s.metrics.Inc("crm_campaign_delivery_handoff_total", "channel", strings.ToLower(d.Channel))
	return true, nil
}

func (s *Service) recordDeliveryConsentCheck(ctx context.Context, tx pgx.Tx, deliveryID, decision string, eligible bool, asOf *time.Time, sourceVersion *int64, checkedAt time.Time) error {
	_, err := tx.Exec(ctx, `UPDATE crm_campaign_deliveries SET
		consent_decision_at_handoff=$2,consent_eligible_at_handoff=$3,consent_checked_at=$4,
		consent_as_of=$5,consent_source_version=$6,policy_checked_at=$4,updated_at=$4 WHERE id=$1`,
		deliveryID, decision, eligible, checkedAt, asOf, sourceVersion)
	return err
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (s *Service) transitionDelivery(ctx context.Context, tx pgx.Tx, d *CampaignDelivery, to, reason, source, eventID string, sourceVersion int64, at time.Time) error {
	from := d.Status
	_, err := tx.Exec(ctx, `UPDATE crm_campaign_deliveries SET status=$2::text,suppression_reason=NULLIF($3::text,''),updated_at=$4::timestamptz,completed_at=CASE WHEN $2::text IN ('SUPPRESSED','DELIVERED','FAILED_PERMANENT','CANCELLED') THEN $4::timestamptz END,version=version+1 WHERE id=$1`, d.ID, to, reason, at)
	if err != nil {
		return err
	}
	if err = s.appendDeliveryHistory(ctx, tx, d.BusinessID, d.ID, &from, to, reason, source, eventID, sourceVersion, at); err != nil {
		return err
	}
	if to == DeliverySuppressed {
		s.metrics.Inc("crm_campaign_delivery_suppressed_total", "reason", strings.ToLower(reason))
		actor := Actor{VendorID: d.BusinessID}
		if err = s.auditTx(ctx, tx, actor, "crm.campaign.delivery.suppressed", "crm_campaign_delivery", d.ID, "", map[string]any{"status": from}, map[string]any{"status": to, "reason": reason}, ""); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) appendDeliveryHistory(ctx context.Context, tx pgx.Tx, businessID, deliveryID string, from *string, to, reason, source, eventID string, sourceVersion int64, at time.Time) error {
	var version any
	if sourceVersion > 0 {
		version = sourceVersion
	}
	_, err := tx.Exec(ctx, `INSERT INTO crm_campaign_delivery_history(id,business_id,delivery_id,from_status,to_status,reason_code,source,source_event_id,source_version,occurred_at) VALUES($1,$2,$3,$4,$5,NULLIF($6,''),$7,NULLIF($8,''),$9,$10)`, `crmdh_`+uuid.NewString(), businessID, deliveryID, from, to, reason, source, eventID, version, at)
	return err
}

func (s *Service) refreshRun(ctx context.Context, tx pgx.Tx, runID string, now time.Time) error {
	var priorStatus, businessID, campaignID string
	if err := tx.QueryRow(ctx, `SELECT status,business_id,campaign_id FROM crm_campaign_runs WHERE id=$1`, runID).Scan(&priorStatus, &businessID, &campaignID); err != nil {
		return err
	}
	var total, handed, delivered, suppressed, failed, active int
	err := tx.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE status IN ('HANDED_OFF','ACCEPTED','DELIVERED')),count(*) FILTER(WHERE status='DELIVERED'),count(*) FILTER(WHERE status='SUPPRESSED'),count(*) FILTER(WHERE status='FAILED_PERMANENT'),count(*) FILTER(WHERE status IN ('PENDING','DEFERRED_QUIET_HOURS','FAILED_TRANSIENT')) FROM crm_campaign_deliveries WHERE run_id=$1`, runID).Scan(&total, &handed, &delivered, &suppressed, &failed, &active)
	if err != nil {
		return err
	}
	var finalStatus string
	var version int64
	err = tx.QueryRow(ctx, `UPDATE crm_campaign_runs SET total_deliveries=$2,handed_off_count=$3,delivered_count=$4,suppressed_count=$5,failed_count=$6,updated_at=$7,status=CASE WHEN status='RUNNING' AND $8=0 THEN CASE WHEN $6>0 THEN 'COMPLETED_WITH_FAILURES' ELSE 'COMPLETED' END ELSE status END,completed_at=CASE WHEN status='RUNNING' AND $8=0 THEN $7 ELSE completed_at END,version=version+1 WHERE id=$1 RETURNING status,version`, runID, total, handed, delivered, suppressed, failed, now, active).Scan(&finalStatus, &version)
	if err != nil {
		return err
	}
	if priorStatus == CampaignRunRunning && (finalStatus == CampaignRunCompleted || finalStatus == CampaignRunCompletedWithFailures) {
		actor := Actor{VendorID: businessID}
		if err = s.auditTx(ctx, tx, actor, "crm.campaign.run.completed", "crm_campaign_run", runID, "", map[string]any{"status": priorStatus}, map[string]any{"status": finalStatus, "handed_off": handed, "suppressed": suppressed, "failed": failed}, ""); err != nil {
			return err
		}
		if err = s.publishTx(ctx, tx, actor, "vendor.crm.campaign.run.completed", "crm_campaign_run", runID, version, map[string]any{"campaign_ref": campaignID, "status": finalStatus, "handed_off_count": handed, "suppressed_count": suppressed, "failed_count": failed}); err != nil {
			return err
		}
		if failed > 0 {
			s.metrics.Inc("crm_campaign_run_failure_total")
		}
	}
	return nil
}

type DeliveryEvent struct {
	EventID, DeliveryRef, Status, ReasonCode string
	SourceVersion                            int64
	OccurredAt                               time.Time
}

var deliveryEventRank = map[string]int{DeliveryHandedOff: 10, DeliveryAccepted: 20, DeliveryFailedRetry: 25, DeliveryFailedFinal: 30, DeliveryDelivered: 40}

// IngestCampaignDeliveryEvent projects authenticated Notification Service
// events. HTTP authentication/replay protection lives at the internal edge;
// this transaction adds durable event-id dedupe and monotonic state ordering.
func (s *Service) IngestCampaignDeliveryEvent(ctx context.Context, source string, event DeliveryEvent) (bool, error) {
	if source != "notification-service" {
		return false, apperrors.Forbidden("This service may not publish campaign delivery events.", nil)
	}
	event.EventID, event.DeliveryRef, event.Status = strings.TrimSpace(event.EventID), strings.TrimSpace(event.DeliveryRef), strings.ToUpper(strings.TrimSpace(event.Status))
	if event.EventID == "" || event.DeliveryRef == "" || event.SourceVersion <= 0 {
		return false, apperrors.BadRequest("Delivery event identity and source version are required.", nil)
	}
	if _, ok := deliveryEventRank[event.Status]; !ok {
		return false, apperrors.BadRequest("Unsupported delivery event status.", nil)
	}
	if event.OccurredAt.IsZero() {
		event.OccurredAt = s.now()
	}
	raw, _ := json.Marshal(event)
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var oldHash string
	err = tx.QueryRow(ctx, `SELECT payload_hash FROM crm_campaign_delivery_event_inbox WHERE event_id=$1`, event.EventID).Scan(&oldHash)
	if err == nil {
		if oldHash != hash {
			return false, apperrors.Conflict("Event id was reused with different content.", nil)
		}
		return false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	d, err := scanCampaignDelivery(tx.QueryRow(ctx, `SELECT `+deliveryCols+` FROM crm_campaign_deliveries WHERE id=$1 FOR UPDATE`, event.DeliveryRef))
	if errors.Is(err, pgx.ErrNoRows) {
		return false, apperrors.NotFound("Campaign delivery not found.", nil)
	}
	if err != nil {
		return false, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO crm_campaign_delivery_event_inbox(event_id,payload_hash,delivery_id,source_service,source_version) VALUES($1,$2,$3,$4,$5)`, event.EventID, hash, d.ID, source, event.SourceVersion); err != nil {
		return false, err
	}
	applied := false
	if event.SourceVersion > d.LastEventVersion && deliveryEventRank[event.Status] >= deliveryEventRank[d.Status] && d.Status != DeliverySuppressed && d.Status != DeliveryCancelled {
		from := d.Status
		_, err = tx.Exec(ctx, `UPDATE crm_campaign_deliveries SET status=$2,last_event_version=$3,updated_at=$4,completed_at=CASE WHEN $2 IN ('DELIVERED','FAILED_PERMANENT') THEN $4 ELSE completed_at END,version=version+1 WHERE id=$1`, d.ID, event.Status, event.SourceVersion, s.now())
		if err != nil {
			return false, err
		}
		if err = s.appendDeliveryHistory(ctx, tx, d.BusinessID, d.ID, &from, event.Status, event.ReasonCode, "NOTIFICATION", event.EventID, event.SourceVersion, event.OccurredAt); err != nil {
			return false, err
		}
		if err = s.refreshRun(ctx, tx, d.RunID, s.now()); err != nil {
			return false, err
		}
		applied = true
		actor := Actor{VendorID: d.BusinessID}
		eventType := "vendor.crm.campaign.delivery.delivered"
		if event.Status == DeliveryFailedFinal {
			eventType = "vendor.crm.campaign.delivery.failed"
		}
		if event.Status == DeliveryDelivered || event.Status == DeliveryFailedFinal {
			if err = s.publishTx(ctx, tx, actor, eventType, "crm_campaign_delivery", d.ID, d.Version+1, map[string]any{"campaign_ref": d.CampaignID, "run_ref": d.RunID, "status": event.Status}); err != nil {
				return false, err
			}
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return false, err
	}
	return applied, nil
}

func (s *Service) ListCampaignDeliveries(ctx context.Context, actor Actor, scope Scope, campaignID, runID string, limit, offset int) ([]CampaignDelivery, bool, error) {
	if _, err := s.GetCampaignRun(ctx, actor, scope, campaignID, runID); err != nil {
		return nil, false, err
	}
	limit, offset = boundPage(limit, offset)
	rows, err := s.db.Query(ctx, `SELECT `+deliveryCols+` FROM crm_campaign_deliveries WHERE business_id=$1 AND run_id=$2 ORDER BY created_at,id LIMIT $3 OFFSET $4`, actor.VendorID, runID, limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := make([]CampaignDelivery, 0, limit)
	for rows.Next() {
		d, e := scanCampaignDelivery(rows)
		if e != nil {
			return nil, false, e
		}
		out = append(out, d)
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, rows.Err()
}
func (s *Service) GetCampaignDelivery(ctx context.Context, actor Actor, scope Scope, campaignID, runID, deliveryID string) (CampaignDelivery, error) {
	if _, err := s.GetCampaignRun(ctx, actor, scope, campaignID, runID); err != nil {
		return CampaignDelivery{}, err
	}
	d, err := scanCampaignDelivery(s.db.QueryRow(ctx, `SELECT `+deliveryCols+` FROM crm_campaign_deliveries WHERE business_id=$1 AND run_id=$2 AND id=$3`, actor.VendorID, runID, deliveryID))
	if errors.Is(err, pgx.ErrNoRows) {
		return CampaignDelivery{}, apperrors.NotFound("Campaign delivery not found.", nil)
	}
	return d, err
}
func (s *Service) CampaignDeliveryHistory(ctx context.Context, actor Actor, scope Scope, campaignID, runID, deliveryID string, limit, offset int) ([]CampaignDeliveryHistory, bool, error) {
	if _, err := s.GetCampaignDelivery(ctx, actor, scope, campaignID, runID, deliveryID); err != nil {
		return nil, false, err
	}
	limit, offset = boundPage(limit, offset)
	rows, err := s.db.Query(ctx, `SELECT id,delivery_id,from_status,to_status,reason_code,source,source_event_id,source_version,occurred_at,recorded_at FROM crm_campaign_delivery_history WHERE business_id=$1 AND delivery_id=$2 ORDER BY recorded_at,id LIMIT $3 OFFSET $4`, actor.VendorID, deliveryID, limit+1, offset)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := make([]CampaignDeliveryHistory, 0, limit)
	for rows.Next() {
		var h CampaignDeliveryHistory
		if err = rows.Scan(&h.ID, &h.DeliveryID, &h.FromStatus, &h.ToStatus, &h.ReasonCode, &h.Source, &h.SourceEventID, &h.SourceVersion, &h.OccurredAt, &h.RecordedAt); err != nil {
			return nil, false, err
		}
		out = append(out, h)
	}
	more := len(out) > limit
	if more {
		out = out[:limit]
	}
	return out, more, rows.Err()
}

// CreateTriggeredCampaignRun is the internal abstraction future authoritative
// events can call. There is deliberately no arbitrary public trigger route.
func (s *Service) CreateTriggeredCampaignRun(ctx context.Context, actor Actor, scope Scope, campaignID, eventID string, definitionVersion int64, in CreateCampaignRunInput) (CampaignRun, error) {
	if strings.TrimSpace(eventID) == "" {
		return CampaignRun{}, fmt.Errorf("trigger event identity is required")
	}
	in.TriggerType = "EVENT"
	in.TriggerRef = eventID
	in.ExactDefinitionVersion = definitionVersion
	if definitionVersion <= 0 {
		return CampaignRun{}, fmt.Errorf("exact campaign definition version is required")
	}
	return s.CreateCampaignRun(ctx, actor, scope, campaignID, in)
}

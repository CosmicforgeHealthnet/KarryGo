package crm

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

// Section 8H — export command surface.
//
// Creation, cancellation and access are three separate authorities because
// they are three separate disclosures: knowing an export exists is not reading
// it, and asking for one is not the same act as releasing its bytes. Step-up
// guards the two that touch customer data.

const exportColumns = `id,business_id,branch_id,scope,requested_by_staff_id,export_type,format,status,
	filter_document,field_set_version,row_limit,row_count,artifact_ref,artifact_bytes,checksum_sha256,
	privacy_fence_at,requested_at,started_at,completed_at,expires_at,cancelled_at,failed_at,error_code,
	worker_attempt_count,version`

func scanExport(row pgx.Row) (Export, error) {
	var e Export
	var filter []byte
	if err := row.Scan(&e.ID, &e.BusinessID, &e.BranchID, &e.Scope, &e.RequestedBy, &e.ExportType,
		&e.Format, &e.Status, &filter, &e.FieldSetVersion, &e.RowLimit, &e.RowCount, &e.ArtifactRef,
		&e.ArtifactBytes, &e.Checksum, &e.PrivacyFenceAt, &e.RequestedAt, &e.StartedAt, &e.CompletedAt,
		&e.ExpiresAt, &e.CancelledAt, &e.FailedAt, &e.ErrorCode, &e.Attempts, &e.Version); err != nil {
		return Export{}, err
	}
	if len(filter) > 0 {
		if err := json.Unmarshal(filter, &e.Filter); err != nil {
			return Export{}, err
		}
	}
	return e, nil
}

// CreateExportInput is one export request.
//
// Notice what is absent: no query, no SQL, no column list, no sort, no table
// and no free-text predicate. A request names a dataset and narrows it along
// dimensions that dataset declares, and that is the entire vocabulary.
type CreateExportInput struct {
	ExportType    string
	Format        string
	ScopeType     string
	BranchRef     string
	RowLimit      int
	Filter        ExportFilter
	StepUpGrantID string
	SessionID     string
}

// CreateExport queues a durable export job.
func (s *Service) CreateExport(ctx context.Context, actor Actor, scope Scope,
	in CreateExportInput) (Export, error) {
	if err := s.assertExportsAvailable(); err != nil {
		return Export{}, err
	}
	if !s.flags.WritesEnabled {
		return Export{}, errWritesDisabled()
	}
	// No store means no artifact can ever exist. Refusing here rather than at
	// generation time is deliberate: queueing work that cannot possibly finish
	// would leave an operator waiting for a file nobody can produce.
	if s.artifacts == nil {
		return Export{}, errExportStorageUnavailable()
	}

	exportType := normalizeExportEnum(in.ExportType)
	dataset, ok := ExportDatasetFor(exportType)
	if !ok {
		return Export{}, crmError(apperrors.Validation("Choose a supported export type.",
			[]apperrors.FieldViolation{{Field: "export_type", Message: "Unsupported export type."}}),
			CodeExportInvalidType)
	}
	if dataset.RequiresFlag != nil && !dataset.RequiresFlag(s.flags) {
		// The domain behind this dataset is switched off, so an empty file
		// would read as "this business has none of these" rather than "we did
		// not look".
		return Export{}, crmError(apperrors.Unavailable(
			"That export type is not available on this deployment.", nil), CodeExportInvalidType)
	}
	format := normalizeExportEnum(in.Format)
	if format == "" {
		format = FormatCSV
	}
	if !ValidExportFormat(format) {
		return Export{}, crmError(apperrors.Validation("Choose a supported format.",
			[]apperrors.FieldViolation{{Field: "format", Message: "Format must be CSV or JSONL."}}),
			CodeExportInvalidFormat)
	}
	limit := in.RowLimit
	if limit <= 0 {
		limit = s.exports.DefaultLimit
	}
	if limit > s.exports.MaxLimit {
		return Export{}, errExportTooLarge()
	}
	filter, err := s.validateExportFilter(ctx, actor, exportType, in.Filter)
	if err != nil {
		return Export{}, err
	}
	scopeType, branchID, err := s.resolveIntelligenceScope(ctx, actor, scope, in.ScopeType, in.BranchRef)
	if err != nil {
		return Export{}, err
	}
	// Step-up is consumed before anything is written, so a refused elevation
	// leaves no trace of a job that never happened.
	if err = s.consumeExportStepUp(ctx, actor, in.SessionID, in.StepUpGrantID, StepUpPurposeExportCreate); err != nil {
		return Export{}, err
	}

	document, err := filter.document()
	if err != nil {
		return Export{}, err
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Export{}, err
	}
	defer tx.Rollback(ctx)

	export, err := scanExport(tx.QueryRow(ctx, `INSERT INTO crm_exports
		(id,business_id,branch_id,scope,requested_by_staff_id,export_type,format,status,
		 filter_document,field_set_version,row_limit,privacy_fence_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,'QUEUED',$8,$9,$10,$11) RETURNING `+exportColumns,
		newID("crmexp_"), actor.VendorID, branchID, scopeType, actor.StaffID, exportType, format,
		document, dataset.FieldSetVersion, limit, s.now()))
	if err != nil {
		return Export{}, err
	}
	if err = s.auditExport(ctx, actor, export, "crm.export.requested", nil); err != nil {
		return Export{}, err
	}
	if err = s.publishExportEvent(ctx, tx, export, "vendor.crm.export.requested", nil); err != nil {
		return Export{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Export{}, err
	}
	s.metrics.Inc("crm_export_requested_total", "export_type", exportType, "format", format)
	return export, nil
}

// validateExportFilter checks a filter against the dataset that will use it.
//
// Every enum is checked against a closed list and every reference is checked
// for existence inside the caller's own tenant, so a filter can neither smuggle
// a value nor confirm that another business's segment exists.
func (s *Service) validateExportFilter(ctx context.Context, actor Actor, exportType string,
	filter ExportFilter) (ExportFilter, error) {
	invalid := func(field, message string) error {
		return crmError(apperrors.Validation("Check the export filter.",
			[]apperrors.FieldViolation{{Field: field, Message: message}}), CodeExportInvalidFilter)
	}
	filter.SegmentRef = strings.TrimSpace(filter.SegmentRef)
	filter.CampaignRef = strings.TrimSpace(filter.CampaignRef)
	filter.ModelKey = normalizeExportEnum(filter.ModelKey)

	if filter.ModelKey != "" && !ValidModelKey(filter.ModelKey) {
		return filter, invalid("filter.model_key", "Unknown model.")
	}
	for index, band := range filter.RiskBands {
		band = normalizeExportEnum(band)
		switch band {
		case RiskLow, RiskMedium, RiskHigh, RiskCritical:
		default:
			return filter, invalid("filter.risk_bands", "Unknown risk band.")
		}
		filter.RiskBands[index] = band
	}
	for index, state := range filter.ScoreStates {
		state = normalizeExportEnum(state)
		switch state {
		case ScoreNotAvailable, ScoreCurrent, ScoreStale, ScoreFailed:
		default:
			return filter, invalid("filter.score_states", "Unknown score state.")
		}
		filter.ScoreStates[index] = state
	}
	for index, status := range filter.Statuses {
		status = normalizeExportEnum(status)
		if !safeToken.MatchString(status) {
			return filter, invalid("filter.statuses", "Unsupported status value.")
		}
		filter.Statuses[index] = status
	}

	switch exportType {
	case ExportSegmentMembers:
		if filter.SegmentRef == "" {
			return filter, invalid("filter.segment_ref", "A segment export must name a segment.")
		}
		var exists bool
		if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM crm_segments
			WHERE business_id=$1 AND id=$2 AND status <> 'ARCHIVED')`,
			actor.VendorID, filter.SegmentRef).Scan(&exists); err != nil {
			return filter, err
		}
		if !exists {
			return filter, crmError(apperrors.NotFound("Segment not found.", nil), CodeExportScopeDenied)
		}
	case ExportCampaignSummary:
		if filter.CampaignRef != "" {
			var exists bool
			if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM crm_campaigns
				WHERE business_id=$1 AND id=$2)`, actor.VendorID, filter.CampaignRef).Scan(&exists); err != nil {
				return filter, err
			}
			if !exists {
				return filter, crmError(apperrors.NotFound("Campaign not found.", nil), CodeExportScopeDenied)
			}
		}
	}
	return filter, nil
}

// consumeExportStepUp spends one elevation.
//
// A nil consumer disables the guarded operation rather than skipping the
// check. An unenforced step-up is worse than an absent feature: it looks like
// protection in every log and diagram while providing none.
func (s *Service) consumeExportStepUp(ctx context.Context, actor Actor, sessionID, grantID, purpose string) error {
	if s.stepUp == nil {
		return errExportStepUpRequired()
	}
	if strings.TrimSpace(grantID) == "" {
		return errExportStepUpRequired()
	}
	if sessionID == "" {
		sessionID = actor.SessionID
	}
	if err := s.stepUp.ConsumeGrant(ctx, grantID, sessionID, actor.StaffID, actor.VendorID, purpose); err != nil {
		return errExportStepUpRequired()
	}
	return nil
}

// ---------------------------------------------------------------------------
// Reads
// ---------------------------------------------------------------------------

// ExportListOptions pages the job list.
type ExportListOptions struct {
	Limit, Offset int
	Status        string
	ExportType    string
}

// ListExports pages the export jobs visible to the caller.
func (s *Service) ListExports(ctx context.Context, actor Actor, scope Scope,
	opts ExportListOptions) ([]Export, bool, error) {
	if err := s.assertExportsAvailable(); err != nil {
		return nil, false, err
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = DefaultPageSize
	}
	if limit > MaxPageSize {
		limit = MaxPageSize
	}
	conditions := []string{"business_id = $1"}
	args := []any{actor.VendorID}
	if !scope.Global {
		// A branch actor sees the branch-scoped jobs of branches they hold, and
		// never a business-wide export — whose very existence, size and row
		// count would describe the whole book.
		args = append(args, scope.BranchIDs)
		conditions = append(conditions, "scope='BRANCH' AND branch_id = ANY($2)")
	}
	if status := normalizeExportEnum(opts.Status); status != "" {
		args = append(args, status)
		conditions = append(conditions, "status = $"+itoa(len(args)))
	}
	if exportType := normalizeExportEnum(opts.ExportType); exportType != "" {
		args = append(args, exportType)
		conditions = append(conditions, "export_type = $"+itoa(len(args)))
	}
	args = append(args, limit+1, opts.Offset)
	rows, err := s.db.Query(ctx, `SELECT `+exportColumns+` FROM crm_exports WHERE `+
		strings.Join(conditions, " AND ")+` ORDER BY requested_at DESC, id DESC
		LIMIT $`+itoa(len(args)-1)+` OFFSET $`+itoa(len(args)), args...)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	out := []Export{}
	for rows.Next() {
		export, err := scanExport(rows)
		if err != nil {
			return nil, false, err
		}
		out = append(out, export)
	}
	if err = rows.Err(); err != nil {
		return nil, false, err
	}
	hasMore := len(out) > limit
	if hasMore {
		out = out[:limit]
	}
	return out, hasMore, nil
}

// GetExport reads one job, refusing anything outside the caller's reach with
// the same answer it gives for a job that does not exist.
func (s *Service) GetExport(ctx context.Context, actor Actor, scope Scope, exportID string) (Export, error) {
	if err := s.assertExportsAvailable(); err != nil {
		return Export{}, err
	}
	export, err := s.loadExport(ctx, s.db, actor.VendorID, exportID)
	if err != nil {
		return Export{}, err
	}
	if !s.exportVisible(scope, export) {
		return Export{}, errExportNotFound()
	}
	return s.expireIfDue(ctx, export)
}

func (s *Service) loadExport(ctx context.Context, q querier, businessID, exportID string) (Export, error) {
	export, err := scanExport(q.QueryRow(ctx, `SELECT `+exportColumns+` FROM crm_exports
		WHERE business_id=$1 AND id=$2`, businessID, exportID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Export{}, errExportNotFound()
	}
	return export, err
}

func (s *Service) exportVisible(scope Scope, export Export) bool {
	if scope.Global {
		return true
	}
	return export.Scope == IntelligenceScopeBranch && export.BranchID != nil &&
		containsString(scope.BranchIDs, *export.BranchID)
}

// expireIfDue retires an artifact whose retention has run out.
//
// It happens on read as well as on the sweeper, so a download attempt can
// never race a cleaner and find a live artifact past its expiry.
func (s *Service) expireIfDue(ctx context.Context, export Export) (Export, error) {
	if export.Status != ExportReady || export.ExpiresAt == nil {
		return export, nil
	}
	// The comparison is made by the database, against the same clock that
	// stamped the expiry. A service clock even slightly behind would keep
	// releasing an artifact whose retention has already elapsed.
	var due bool
	if err := s.db.QueryRow(ctx, `SELECT expires_at <= now() FROM crm_exports
		WHERE id=$1 AND business_id=$2`, export.ID, export.BusinessID).Scan(&due); err != nil {
		return export, err
	}
	if !due {
		return export, nil
	}
	return s.expireExport(ctx, export)
}

func (s *Service) expireExport(ctx context.Context, export Export) (Export, error) {
	if export.ArtifactRef != nil && s.artifacts != nil {
		// Best effort: the job must reach EXPIRED even if the store is briefly
		// unreachable, because leaving it READY would keep it downloadable.
		_ = s.artifacts.Delete(ctx, export.BusinessID, *export.ArtifactRef, "expired")
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return export, err
	}
	defer tx.Rollback(ctx)
	updated, err := scanExport(tx.QueryRow(ctx, `UPDATE crm_exports SET status='EXPIRED',
		artifact_ref=NULL, updated_at=now(), version=version+1
		WHERE id=$1 AND business_id=$2 AND status='READY' RETURNING `+exportColumns,
		export.ID, export.BusinessID))
	if errors.Is(err, pgx.ErrNoRows) {
		return s.loadExport(ctx, s.db, export.BusinessID, export.ID)
	}
	if err != nil {
		return export, err
	}
	if err = s.auditExport(ctx, Actor{VendorID: export.BusinessID, StaffID: "export-generator"},
		updated, "crm.export.expired", nil); err != nil {
		return export, err
	}
	if err = s.publishExportEvent(ctx, tx, updated, "vendor.crm.export.expired", nil); err != nil {
		return export, err
	}
	return updated, tx.Commit(ctx)
}

// ---------------------------------------------------------------------------
// Cancel
// ---------------------------------------------------------------------------

// CancelExport stops a job and revokes any artifact it produced.
func (s *Service) CancelExport(ctx context.Context, actor Actor, scope Scope,
	exportID string, expectedVersion int64) (Export, error) {
	if err := s.assertExportsAvailable(); err != nil {
		return Export{}, err
	}
	if !s.flags.WritesEnabled {
		return Export{}, errWritesDisabled()
	}
	export, err := s.loadExport(ctx, s.db, actor.VendorID, exportID)
	if err != nil {
		return Export{}, err
	}
	if !s.exportVisible(scope, export) {
		return Export{}, errExportNotFound()
	}
	if expectedVersion > 0 && export.Version != expectedVersion {
		return Export{}, errExportVersionConflict()
	}
	switch export.Status {
	case ExportCancelled:
		return export, nil
	case ExportExpired, ExportFailed:
		return Export{}, crmError(apperrors.Conflict("That export can no longer be cancelled.", nil),
			CodeExportCancelled)
	}

	// The artifact goes first. If the store call fails the job stays as it was,
	// which is recoverable; the reverse — a CANCELLED job whose bytes are still
	// downloadable — is not.
	if export.ArtifactRef != nil && s.artifacts != nil {
		if err = s.artifacts.Delete(ctx, export.BusinessID, *export.ArtifactRef, "cancelled"); err != nil {
			return Export{}, errExportStorageUnavailable()
		}
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Export{}, err
	}
	defer tx.Rollback(ctx)
	// The status guard is the race fence against the worker: a generator that
	// finishes concurrently cannot promote a job this transaction cancelled,
	// because publish requires status QUEUED or RUNNING.
	updated, err := scanExport(tx.QueryRow(ctx, `UPDATE crm_exports SET status='CANCELLED',
		cancelled_at=now(), artifact_ref=NULL, lease_owner=NULL, lease_expires_at=NULL,
		updated_at=now(), version=version+1
		WHERE id=$1 AND business_id=$2 AND status IN ('QUEUED','RUNNING','READY')
		RETURNING `+exportColumns, export.ID, export.BusinessID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Export{}, crmError(apperrors.Conflict("That export can no longer be cancelled.", nil),
			CodeExportCancelled)
	}
	if err != nil {
		return Export{}, err
	}
	if err = s.auditExport(ctx, actor, updated, "crm.export.cancelled", nil); err != nil {
		return Export{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Export{}, err
	}
	return updated, nil
}

// ---------------------------------------------------------------------------
// Download
// ---------------------------------------------------------------------------

// ExportAccessInput carries the elevation for a download.
type ExportAccessInput struct {
	StepUpGrantID string
	SessionID     string
}

// AuthorizeExportDownload releases short-lived access to a completed artifact.
//
// The order of checks is the security design: tenancy, then branch scope, then
// readiness, then expiry, then a fresh step-up, and only then does the private
// store issue a token. Nothing durable is handed back — no URL is stored, and
// the grant the caller receives outlives neither the request nor the store's
// own short window.
func (s *Service) AuthorizeExportDownload(ctx context.Context, actor Actor, scope Scope,
	exportID string, in ExportAccessInput) (ArtifactAccess, Export, error) {
	if err := s.assertExportsAvailable(); err != nil {
		return ArtifactAccess{}, Export{}, err
	}
	export, err := s.loadExport(ctx, s.db, actor.VendorID, exportID)
	if err != nil {
		return ArtifactAccess{}, Export{}, err
	}
	if !s.exportVisible(scope, export) {
		return ArtifactAccess{}, Export{}, errExportNotFound()
	}
	if export, err = s.expireIfDue(ctx, export); err != nil {
		return ArtifactAccess{}, Export{}, err
	}
	switch export.Status {
	case ExportExpired:
		return ArtifactAccess{}, Export{}, errExportExpired()
	case ExportReady:
	default:
		return ArtifactAccess{}, Export{}, errExportNotReady()
	}
	if export.ArtifactRef == nil {
		return ArtifactAccess{}, Export{}, errExportNotReady()
	}
	if err = s.consumeExportStepUp(ctx, actor, in.SessionID, in.StepUpGrantID,
		StepUpPurposeExportDownload); err != nil {
		return ArtifactAccess{}, Export{}, err
	}
	if s.artifacts == nil {
		return ArtifactAccess{}, Export{}, errExportStorageUnavailable()
	}
	access, err := s.artifacts.Access(ctx, export.BusinessID, *export.ArtifactRef)
	if err != nil {
		return ArtifactAccess{}, Export{}, errExportStorageUnavailable()
	}
	// The audit records that access was granted, never what was in the file.
	if err = s.auditExport(ctx, actor, export, "crm.export.downloaded",
		map[string]any{"row_count": export.RowCount}); err != nil {
		return ArtifactAccess{}, Export{}, err
	}
	s.metrics.Inc("crm_export_download_total", "export_type", export.ExportType)
	return access, export, nil
}

// ---------------------------------------------------------------------------
// Audit and events
// ---------------------------------------------------------------------------

// auditExport records an export lifecycle event.
//
// The filter document is deliberately not logged: it can name a segment or a
// campaign, and an audit trail is not the place to accumulate a second copy of
// who was targeted. Row counts and identifiers are enough to reconstruct what
// happened.
func (s *Service) auditExport(ctx context.Context, actor Actor, export Export, action string,
	extra map[string]any) error {
	after := map[string]any{
		"export_type": export.ExportType, "format": export.Format, "status": export.Status,
		"scope": export.Scope, "row_count": export.RowCount, "row_limit": export.RowLimit,
	}
	if export.BranchID != nil {
		after["branch_ref"] = *export.BranchID
	}
	if export.CompletedAt != nil {
		after["completed_at"] = *export.CompletedAt
	}
	if export.ExpiresAt != nil {
		after["expires_at"] = *export.ExpiresAt
	}
	for key, value := range extra {
		after[key] = value
	}
	return s.audit.Record(ctx, AuditEntry{
		ActorID: actor.StaffID, Action: action, AggregateType: "crm_export",
		AggregateID: export.ID, VendorID: export.BusinessID, BranchID: derefOr(export.BranchID),
		After: after,
	})
}

// publishExportEvent emits the outbox event.
//
// The payload carries identity and counts. It never carries a row, a filter, a
// download token or an artifact location: an event bus is the last place a
// customer export should be reconstructable from.
func (s *Service) publishExportEvent(ctx context.Context, tx pgx.Tx, export Export,
	eventType string, extra map[string]any) error {
	if s.events == nil {
		return nil
	}
	payload := map[string]any{
		"export_ref": export.ID, "export_type": export.ExportType, "format": export.Format,
		"scope": export.Scope, "status": export.Status, "row_count": export.RowCount,
	}
	if export.BranchID != nil {
		payload["branch_ref"] = *export.BranchID
	}
	for key, value := range extra {
		payload[key] = value
	}
	return s.events.Publish(ctx, tx, eventType, "crm_export", export.ID, export.Version,
		export.BusinessID, "", "", payload)
}

// ExportRetentionPass expires artifacts whose retention has elapsed. It is the
// cleanup half of the export worker.
func (s *Service) ExportRetentionPass(ctx context.Context, limit int) (int, error) {
	if !s.flags.CRMEnabled || !s.flags.ExportsEnabled {
		return 0, nil
	}
	if limit <= 0 {
		limit = 50
	}
	// The due test uses the database clock, the same one that stamped the
	// expiry, so a sweeper on a slightly slow host cannot leave an overdue
	// artifact downloadable.
	rows, err := s.db.Query(ctx, `SELECT `+exportColumns+` FROM crm_exports
		WHERE status='READY' AND expires_at IS NOT NULL AND expires_at <= now()
		ORDER BY expires_at LIMIT $1`, limit)
	if err != nil {
		return 0, err
	}
	due := []Export{}
	for rows.Next() {
		export, err := scanExport(rows)
		if err != nil {
			rows.Close()
			return 0, err
		}
		due = append(due, export)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return 0, err
	}
	expired := 0
	for _, export := range due {
		if _, err = s.expireExport(ctx, export); err != nil {
			return expired, err
		}
		expired++
	}
	return expired, nil
}

var _ = time.Time{}

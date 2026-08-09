package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/cursor"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

const slaColumns = `id, organization_id, customer_id, COALESCE(contract_id, ''),
	COALESCE(future_service_catalog_item_id, ''), metric_code,
	trim_scale(target_value)::text,
	measurement_unit, measurement_window, escalation_policy, effective_from, effective_to,
	status, version, created_by, updated_by, created_at, updated_at`

func scanSLA(row pgx.Row) (models.SLA, error) {
	var sla models.SLA
	var policy []byte
	err := row.Scan(&sla.ID, &sla.OrganizationID, &sla.CustomerID, &sla.ContractID,
		&sla.FutureServiceCatalogItemID, &sla.MetricCode, &sla.TargetValue,
		&sla.MeasurementUnit, &sla.MeasurementWindow, &policy, &sla.EffectiveFrom,
		&sla.EffectiveTo, &sla.Status, &sla.Version, &sla.CreatedBy, &sla.UpdatedBy,
		&sla.CreatedAt, &sla.UpdatedAt)
	if err != nil {
		return sla, scanErr(err)
	}
	sla.EscalationPolicy = decodeJSONMap(policy)
	return sla, nil
}

// CreateSLA inserts a draft service level agreement.
func (r *Repository) CreateSLA(ctx context.Context, q database.Querier,
	sla models.SLA) (models.SLA, error) {
	policy, err := encodeJSON(sla.EscalationPolicy)
	if err != nil {
		return models.SLA{}, err
	}
	row := r.q(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_service_level_agreements
			(id, organization_id, customer_id, contract_id, future_service_catalog_item_id,
			 metric_code, target_value, measurement_unit, measurement_window,
			 escalation_policy, effective_from, effective_to, status, created_by, updated_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7::numeric, $8, $9, $10, $11, $12, $13, $14, $14)
		RETURNING `+slaColumns,
		sla.ID, sla.OrganizationID, sla.CustomerID, nullable(sla.ContractID),
		nullable(sla.FutureServiceCatalogItemID), sla.MetricCode, sla.TargetValue,
		sla.MeasurementUnit, sla.MeasurementWindow, policy, sla.EffectiveFrom,
		sla.EffectiveTo, sla.Status, sla.CreatedBy)
	created, err := scanSLA(row)
	if err != nil {
		return created, classify(err, "create sla")
	}
	return created, nil
}

// GetSLA reads one SLA inside the organization.
func (r *Repository) GetSLA(ctx context.Context, q database.Querier,
	organizationID, slaID string) (models.SLA, error) {
	return scanSLA(r.q(q).QueryRow(ctx, `SELECT `+slaColumns+`
		FROM fleet_partner_service_level_agreements WHERE organization_id = $1 AND id = $2`,
		organizationID, slaID))
}

// LockSLA reads one SLA FOR UPDATE.
func (r *Repository) LockSLA(ctx context.Context, q database.Querier,
	organizationID, slaID string) (models.SLA, error) {
	return scanSLA(r.q(q).QueryRow(ctx, `SELECT `+slaColumns+`
		FROM fleet_partner_service_level_agreements WHERE organization_id = $1 AND id = $2
		FOR UPDATE`, organizationID, slaID))
}

// SLAFilter is the allowlisted SLA list query.
type SLAFilter struct {
	Status     string
	CustomerID string
	ContractID string
	MetricCode string
	Cursor     cursor.Cursor
	HasCursor  bool
	Limit      int
}

// ListSLAs returns a tenant-scoped page, newest first.
func (r *Repository) ListSLAs(ctx context.Context, organizationID string,
	filter SLAFilter) ([]models.SLA, error) {
	args := []any{organizationID, filter.Status, filter.CustomerID, filter.ContractID,
		filter.MetricCode}
	where := `organization_id = $1
		AND ($2 = '' OR status = $2)
		AND ($3 = '' OR customer_id = $3)
		AND ($4 = '' OR contract_id = $4)
		AND ($5 = '' OR metric_code = $5)`
	where, args = pageWhere(where, args, filter.Cursor, filter.HasCursor, "created_at")
	args = append(args, cursor.ClampLimit(filter.Limit))

	rows, err := r.db.Query(ctx, `SELECT `+slaColumns+`
		FROM fleet_partner_service_level_agreements WHERE `+where+`
		ORDER BY created_at DESC, id DESC LIMIT $`+itoa(len(args)), args...)
	if err != nil {
		return nil, classify(err, "list slas")
	}
	defer rows.Close()
	out := make([]models.SLA, 0)
	for rows.Next() {
		value, scanError := scanSLA(rows)
		if scanError != nil {
			return nil, scanError
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// SLAUpdate carries the mutable draft SLA fields.
type SLAUpdate struct {
	TargetValue       *string
	MeasurementUnit   *string
	MeasurementWindow *string
	EscalationPolicy  models.JSONMap
	EffectiveFrom     *time.Time
	EffectiveTo       *time.Time
	UpdatedBy         string
}

// UpdateSLA applies a compare-and-swap update to a draft SLA.
//
// An active SLA is a promise a customer is already relying on, so the target
// and window are only editable while it is still a draft. Changing an active
// commitment goes through supersession.
func (r *Repository) UpdateSLA(ctx context.Context, q database.Querier,
	organizationID, slaID string, expectedVersion int64,
	update SLAUpdate) (models.SLA, error) {
	var policy any
	if update.EscalationPolicy != nil {
		encoded, err := encodeJSON(update.EscalationPolicy)
		if err != nil {
			return models.SLA{}, err
		}
		policy = encoded
	}
	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_service_level_agreements SET
			target_value = COALESCE($4::numeric, target_value),
			measurement_unit = COALESCE($5, measurement_unit),
			measurement_window = COALESCE($6, measurement_window),
			escalation_policy = COALESCE($7::jsonb, escalation_policy),
			effective_from = COALESCE($8, effective_from),
			effective_to = COALESCE($9, effective_to),
			updated_by = $10, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $3 AND status = 'DRAFT'
		RETURNING `+slaColumns,
		organizationID, slaID, expectedVersion, update.TargetValue, update.MeasurementUnit,
		update.MeasurementWindow, policy, update.EffectiveFrom, update.EffectiveTo,
		update.UpdatedBy)
	updated, err := scanSLA(row)
	if err != nil {
		return updated, r.classifySLAWrite(ctx, q, organizationID, slaID, err, "update sla")
	}
	return updated, nil
}

// TransitionSLA moves an SLA to a new status.
func (r *Repository) TransitionSLA(ctx context.Context, q database.Querier,
	organizationID, slaID string, expectedVersion int64,
	fromStatus, toStatus, actor string) (models.SLA, error) {
	row := r.q(q).QueryRow(ctx, `
		UPDATE fleet_partner_service_level_agreements SET
			status = $5, updated_by = $6, version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $3 AND status = $4
		RETURNING `+slaColumns,
		organizationID, slaID, expectedVersion, fromStatus, toStatus, actor)
	updated, err := scanSLA(row)
	if err != nil {
		return updated, r.classifySLAWrite(ctx, q, organizationID, slaID, err,
			"transition sla")
	}
	return updated, nil
}

func (r *Repository) classifySLAWrite(ctx context.Context, q database.Querier,
	organizationID, slaID string, err error, operation string) error {
	classified := classify(err, operation)
	if classified != ErrNotFound {
		return classified
	}
	if _, getErr := r.GetSLA(ctx, q, organizationID, slaID); getErr == nil {
		return ErrVersionConflict
	}
	return ErrNotFound
}

// AppendSLAHistory writes one immutable SLA status change.
func (r *Repository) AppendSLAHistory(ctx context.Context, q database.Querier,
	entry models.SLAHistory) error {
	if entry.ID == "" {
		entry.ID = ids.New(PrefixHistory)
	}
	_, err := r.q(q).Exec(ctx, `
		INSERT INTO fleet_partner_sla_history
			(id, organization_id, sla_id, previous_status, new_status, reason_code,
			 approval_request_id, actor_type, actor_id, correlation_id, causation_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		entry.ID, entry.OrganizationID, entry.SLAID, entry.PreviousStatus, entry.NewStatus,
		entry.ReasonCode, nullable(entry.ApprovalRequestID), entry.ActorType, entry.ActorID,
		entry.CorrelationID, entry.CausationID)
	return classify(err, "append sla history")
}

// ListSLAHistory returns a page of SLA history, newest first.
func (r *Repository) ListSLAHistory(ctx context.Context, organizationID, slaID string,
	page cursor.Cursor, hasCursor bool, limit int) ([]models.SLAHistory, error) {
	args := []any{organizationID, slaID}
	where := `organization_id = $1 AND sla_id = $2`
	where, args = pageWhere(where, args, page, hasCursor, "occurred_at")
	args = append(args, cursor.ClampLimit(limit))

	rows, err := r.db.Query(ctx, `
		SELECT id, organization_id, sla_id, previous_status, new_status, reason_code,
		       COALESCE(approval_request_id, ''), actor_type, actor_id, correlation_id,
		       causation_id, occurred_at
		FROM fleet_partner_sla_history WHERE `+where+`
		ORDER BY occurred_at DESC, id DESC LIMIT $`+itoa(len(args)), args...)
	if err != nil {
		return nil, classify(err, "list sla history")
	}
	defer rows.Close()
	out := make([]models.SLAHistory, 0)
	for rows.Next() {
		var entry models.SLAHistory
		if scanError := rows.Scan(&entry.ID, &entry.OrganizationID, &entry.SLAID,
			&entry.PreviousStatus, &entry.NewStatus, &entry.ReasonCode,
			&entry.ApprovalRequestID, &entry.ActorType, &entry.ActorID, &entry.CorrelationID,
			&entry.CausationID, &entry.OccurredAt); scanError != nil {
			return nil, scanError
		}
		out = append(out, entry)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------------------
// Observations
// ---------------------------------------------------------------------------

const observationColumns = `id, organization_id, sla_id, source_type, source_reference,
	trim_scale(measured_value)::text, measurement_unit, measured_at, window_start, window_end, outcome,
	source_version, event_id, payload_hash, correlation_id, causation_id, created_at`

func scanObservation(row pgx.Row) (models.SLAObservation, error) {
	var observation models.SLAObservation
	err := row.Scan(&observation.ID, &observation.OrganizationID, &observation.SLAID,
		&observation.SourceType, &observation.SourceReference, &observation.MeasuredValue,
		&observation.MeasurementUnit, &observation.MeasuredAt, &observation.WindowStart,
		&observation.WindowEnd, &observation.Outcome, &observation.SourceVersion,
		&observation.EventID, &observation.PayloadHash, &observation.CorrelationID,
		&observation.CausationID, &observation.CreatedAt)
	if err != nil {
		return observation, scanErr(err)
	}
	return observation, nil
}

// ErrObservationConflict reports a same-version observation with a changed
// payload.
var ErrObservationConflict = errors.New("crm: sla observation conflicts with a stored version")

// AppendObservation inserts an append-only SLA measurement.
//
// The unique index on (sla_id, source_type, source_reference, source_version)
// carries the whole idempotency contract. A second delivery of the same version
// is a duplicate when the payload hash matches — which returns the stored row —
// and a conflict when it does not, which is refused. A source that changes its
// mind about a window it already reported must say so with a new version.
func (r *Repository) AppendObservation(ctx context.Context, q database.Querier,
	observation models.SLAObservation) (models.SLAObservation, bool, error) {
	if observation.ID == "" {
		observation.ID = ids.New(PrefixObservation)
	}
	row := r.q(q).QueryRow(ctx, `
		INSERT INTO fleet_partner_sla_observations
			(id, organization_id, sla_id, source_type, source_reference, measured_value,
			 measurement_unit, measured_at, window_start, window_end, outcome, source_version,
			 event_id, payload_hash, correlation_id, causation_id)
		VALUES ($1, $2, $3, $4, $5, $6::numeric, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
		ON CONFLICT (sla_id, source_type, source_reference, source_version) DO NOTHING
		RETURNING `+observationColumns,
		observation.ID, observation.OrganizationID, observation.SLAID, observation.SourceType,
		observation.SourceReference, observation.MeasuredValue, observation.MeasurementUnit,
		observation.MeasuredAt, observation.WindowStart, observation.WindowEnd,
		observation.Outcome, observation.SourceVersion, observation.EventID,
		observation.PayloadHash, observation.CorrelationID, observation.CausationID)
	created, err := scanObservation(row)
	if err == nil {
		return created, false, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return created, false, classify(err, "append observation")
	}

	// DO NOTHING returned no row, so one already exists for this source
	// version. Whether that is a replay or a contradiction depends entirely on
	// the payload hash — and because the insert conflicted rather than failed,
	// the caller's transaction is still usable for this read.
	existing, findErr := r.FindObservation(ctx, q, observation.SLAID, observation.SourceType,
		observation.SourceReference, observation.SourceVersion)
	if findErr != nil {
		return created, false, findErr
	}
	if existing.PayloadHash != observation.PayloadHash {
		return existing, false, ErrObservationConflict
	}
	return existing, true, nil
}

// FindObservation reads the stored observation for a source version.
func (r *Repository) FindObservation(ctx context.Context, q database.Querier,
	slaID, sourceType, sourceReference string,
	sourceVersion int64) (models.SLAObservation, error) {
	return scanObservation(r.q(q).QueryRow(ctx, `SELECT `+observationColumns+`
		FROM fleet_partner_sla_observations
		WHERE sla_id = $1 AND source_type = $2 AND source_reference = $3
		  AND source_version = $4`,
		slaID, sourceType, sourceReference, sourceVersion))
}

// LatestObservationVersion reports the highest source version stored for a
// source reference.
//
// A gap between this and an incoming version means measurements were missed.
// The caller records UNKNOWN rather than interpolating: an unmeasured window is
// not a met one.
func (r *Repository) LatestObservationVersion(ctx context.Context, q database.Querier,
	slaID, sourceType, sourceReference string) (int64, error) {
	var latest *int64
	err := r.q(q).QueryRow(ctx, `
		SELECT max(source_version) FROM fleet_partner_sla_observations
		WHERE sla_id = $1 AND source_type = $2 AND source_reference = $3`,
		slaID, sourceType, sourceReference).Scan(&latest)
	if err != nil {
		return 0, classify(err, "latest observation version")
	}
	if latest == nil {
		return 0, nil
	}
	return *latest, nil
}

// ListObservations returns a page of SLA observations, newest first.
func (r *Repository) ListObservations(ctx context.Context, organizationID, slaID string,
	outcome string, page cursor.Cursor, hasCursor bool,
	limit int) ([]models.SLAObservation, error) {
	args := []any{organizationID, slaID, outcome}
	where := `organization_id = $1 AND sla_id = $2 AND ($3 = '' OR outcome = $3)`
	where, args = pageWhere(where, args, page, hasCursor, "measured_at")
	args = append(args, cursor.ClampLimit(limit))

	rows, err := r.db.Query(ctx, `SELECT `+observationColumns+`
		FROM fleet_partner_sla_observations WHERE `+where+`
		ORDER BY measured_at DESC, id DESC LIMIT $`+itoa(len(args)), args...)
	if err != nil {
		return nil, classify(err, "list observations")
	}
	defer rows.Close()
	out := make([]models.SLAObservation, 0)
	for rows.Next() {
		value, scanError := scanObservation(rows)
		if scanError != nil {
			return nil, scanError
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// ClaimUnevaluatedObservations returns observations awaiting evaluation.
func (r *Repository) ClaimUnevaluatedObservations(ctx context.Context,
	limit int) ([]models.SLAObservation, error) {
	rows, err := r.db.Query(ctx, `SELECT `+observationColumns+`
		FROM fleet_partner_sla_observations
		WHERE outcome = 'NOT_EVALUATED'
		ORDER BY created_at ASC, id ASC LIMIT $1`, cursor.ClampLimit(limit))
	if err != nil {
		return nil, classify(err, "claim unevaluated observations")
	}
	defer rows.Close()
	out := make([]models.SLAObservation, 0)
	for rows.Next() {
		value, scanError := scanObservation(rows)
		if scanError != nil {
			return nil, scanError
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// ClaimMissedObservations returns recent missed outcomes needing escalation.
func (r *Repository) ClaimMissedObservations(ctx context.Context, since time.Time,
	limit int) ([]models.SLAObservation, error) {
	rows, err := r.db.Query(ctx, `SELECT `+observationColumns+`
		FROM fleet_partner_sla_observations
		WHERE outcome = 'MISSED' AND created_at >= $1
		ORDER BY created_at ASC, id ASC LIMIT $2`, since, cursor.ClampLimit(limit))
	if err != nil {
		return nil, classify(err, "claim missed observations")
	}
	defer rows.Close()
	out := make([]models.SLAObservation, 0)
	for rows.Next() {
		value, scanError := scanObservation(rows)
		if scanError != nil {
			return nil, scanError
		}
		out = append(out, value)
	}
	return out, rows.Err()
}

// CountObservationsByOutcome reports per-outcome totals for metrics.
func (r *Repository) CountObservationsByOutcome(ctx context.Context) (map[string]int64, error) {
	rows, err := r.db.Query(ctx, `SELECT outcome, count(*)
		FROM fleet_partner_sla_observations GROUP BY outcome`)
	if err != nil {
		return nil, classify(err, "count observations")
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var outcome string
		var total int64
		if scanError := rows.Scan(&outcome, &total); scanError != nil {
			return nil, scanError
		}
		out[outcome] = total
	}
	return out, rows.Err()
}

// CountSLAs reports the total SLA definition count for metrics.
func (r *Repository) CountSLAs(ctx context.Context) (int64, error) {
	var total int64
	err := r.db.QueryRow(ctx,
		`SELECT count(*) FROM fleet_partner_service_level_agreements`).Scan(&total)
	return total, classify(err, "count slas")
}

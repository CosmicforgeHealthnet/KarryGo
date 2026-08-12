// Package repositories persists private provider decisions and execution operations.
package repositories

import (
	"context"
	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/privateoperations/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"time"
)

var (
	ErrNotFound = errors.New("private operations: not found")
	ErrConflict = errors.New("private operations: conflict")
)

const (
	PrefixAssignment = "pas"
	PrefixDecision   = "pad"
	PrefixHistory    = "pah"
	PrefixExecution  = "pex"
	PrefixAttempt    = "pea"
)

type Repository struct{ db database.Querier }

func NewRepository(db database.Querier) *Repository { return &Repository{db} }
func (r *Repository) q(q database.Querier) database.Querier {
	if q != nil {
		return q
	}
	return r.db
}

const acols = `id,organization_id,branch_id,work_origin,COALESCE(private_order_id,''),COALESCE(shipment_id,''),COALESCE(route_stop_id,''),provider_membership_id,provider_id,vehicle_or_truck_reference,execution_service,status,response_deadline,accepted_at,declined_at,expired_at,cancelled_at,decline_reason,decline_note,COALESCE(replacement_assignment_id,''),version,created_by,created_at,updated_at`

func scanA(row pgx.Row) (models.Assignment, error) {
	var a models.Assignment
	e := row.Scan(&a.ID, &a.OrganizationID, &a.BranchID, &a.WorkOrigin, &a.PrivateOrderID, &a.ShipmentID, &a.RouteStopID, &a.ProviderMembershipID, &a.ProviderID, &a.VehicleOrTruckReference, &a.ExecutionService, &a.Status, &a.ResponseDeadline, &a.AcceptedAt, &a.DeclinedAt, &a.ExpiredAt, &a.CancelledAt, &a.DeclineReason, &a.DeclineNote, &a.ReplacementAssignmentID, &a.Version, &a.CreatedBy, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return a, ErrNotFound
	}
	return a, e
}
func (r *Repository) CreateAssignment(ctx context.Context, q database.Querier, a models.Assignment) (models.Assignment, error) {
	v, e := scanA(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_private_assignments(id,organization_id,branch_id,work_origin,private_order_id,shipment_id,route_stop_id,provider_membership_id,provider_id,vehicle_or_truck_reference,execution_service,status,response_deadline,created_by)VALUES($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),$8,$9,$10,$11,$12,$13,$14)RETURNING `+acols, a.ID, a.OrganizationID, a.BranchID, a.WorkOrigin, a.PrivateOrderID, a.ShipmentID, a.RouteStopID, a.ProviderMembershipID, a.ProviderID, a.VehicleOrTruckReference, a.ExecutionService, a.Status, a.ResponseDeadline, a.CreatedBy))
	if e != nil {
		var pe *pgconn.PgError
		if errors.As(e, &pe) && pe.Code == "23505" {
			return v, ErrConflict
		}
	}
	return v, e
}
func (r *Repository) GetAssignment(ctx context.Context, q database.Querier, org, id string) (models.Assignment, error) {
	return scanA(r.q(q).QueryRow(ctx, `SELECT `+acols+` FROM fleet_partner_private_assignments WHERE organization_id=$1 AND id=$2`, org, id))
}

func (r *Repository) GetAssignmentByID(ctx context.Context, q database.Querier, id string) (models.Assignment, error) {
	return scanA(r.q(q).QueryRow(ctx, `SELECT `+acols+` FROM fleet_partner_private_assignments WHERE id=$1`, id))
}
func (r *Repository) LockAssignment(ctx context.Context, q database.Querier, org, id string) (models.Assignment, error) {
	return scanA(r.q(q).QueryRow(ctx, `SELECT `+acols+` FROM fleet_partner_private_assignments WHERE organization_id=$1 AND id=$2 FOR UPDATE`, org, id))
}
func (r *Repository) ListAssignments(ctx context.Context, org, status string) ([]models.Assignment, error) {
	rows, e := r.db.Query(ctx, `SELECT `+acols+` FROM fleet_partner_private_assignments WHERE organization_id=$1 AND($2='' OR status=$2)ORDER BY created_at DESC LIMIT 100`, org, status)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []models.Assignment
	for rows.Next() {
		a, x := scanA(rows)
		if x != nil {
			return nil, x
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
func (r *Repository) SetAssignmentStatus(ctx context.Context, q database.Querier, org, id string, version int64, status, reason, note, replacement string) (models.Assignment, error) {
	a, e := scanA(r.q(q).QueryRow(ctx, `UPDATE fleet_partner_private_assignments SET status=$1,decline_reason=$2,decline_note=$3,replacement_assignment_id=NULLIF($4,''),accepted_at=CASE WHEN $1='ACCEPTED' THEN now() ELSE accepted_at END,declined_at=CASE WHEN $1='DECLINED' THEN now() ELSE declined_at END,expired_at=CASE WHEN $1='EXPIRED' THEN now() ELSE expired_at END,cancelled_at=CASE WHEN $1='CANCELLED' THEN now() ELSE cancelled_at END,version=version+1,updated_at=now() WHERE organization_id=$5 AND id=$6 AND version=$7 RETURNING `+acols, status, reason, note, replacement, org, id, version))
	if errors.Is(e, ErrNotFound) {
		return a, ErrConflict
	}
	return a, e
}
func (r *Repository) CreateDecision(ctx context.Context, q database.Querier, d models.Decision) (models.Decision, error) {
	d.ID = ids.New(PrefixDecision)
	e := r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_private_assignment_decisions(id,organization_id,assignment_id,provider_id,decision,reason_code,safe_note,safety_protected,idempotency_key,request_fingerprint)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)RETURNING decided_at`, d.ID, d.OrganizationID, d.AssignmentID, d.ProviderID, d.Decision, d.ReasonCode, d.SafeNote, d.SafetyProtected, d.IdempotencyKey, d.RequestFingerprint).Scan(&d.DecidedAt)
	if e != nil {
		var pe *pgconn.PgError
		if errors.As(e, &pe) && pe.Code == "23505" {
			return d, ErrConflict
		}
	}
	return d, e
}
func (r *Repository) GetDecision(ctx context.Context, q database.Querier, org, assignment string) (models.Decision, error) {
	var d models.Decision
	e := r.q(q).QueryRow(ctx, `SELECT id,organization_id,assignment_id,provider_id,decision,reason_code,safe_note,safety_protected,idempotency_key,request_fingerprint,decided_at FROM fleet_partner_private_assignment_decisions WHERE organization_id=$1 AND assignment_id=$2`, org, assignment).Scan(&d.ID, &d.OrganizationID, &d.AssignmentID, &d.ProviderID, &d.Decision, &d.ReasonCode, &d.SafeNote, &d.SafetyProtected, &d.IdempotencyKey, &d.RequestFingerprint, &d.DecidedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	return d, e
}

func (r *Repository) GetDecisionByAssignment(ctx context.Context, q database.Querier, assignment string) (models.Decision, error) {
	var d models.Decision
	e := r.q(q).QueryRow(ctx, `SELECT id,organization_id,assignment_id,provider_id,decision,reason_code,safe_note,safety_protected,idempotency_key,request_fingerprint,decided_at FROM fleet_partner_private_assignment_decisions WHERE assignment_id=$1`, assignment).Scan(&d.ID, &d.OrganizationID, &d.AssignmentID, &d.ProviderID, &d.Decision, &d.ReasonCode, &d.SafeNote, &d.SafetyProtected, &d.IdempotencyKey, &d.RequestFingerprint, &d.DecidedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return d, ErrNotFound
	}
	return d, e
}
func (r *Repository) History(ctx context.Context, q database.Querier, org, id, from, to, reason, actorType, actor, corr string) error {
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_private_assignment_history(id,organization_id,assignment_id,previous_status,new_status,reason_code,actor_type,actor_reference,correlation_id)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, ids.New(PrefixHistory), org, id, from, to, reason, actorType, actor, corr)
	return e
}
func (r *Repository) ClaimExpired(ctx context.Context, q database.Querier, limit int) ([]models.Assignment, error) {
	rows, e := r.q(q).Query(ctx, `SELECT `+acols+` FROM fleet_partner_private_assignments WHERE status='PENDING_PROVIDER_RESPONSE' AND response_deadline<=now() ORDER BY response_deadline FOR UPDATE SKIP LOCKED LIMIT $1`, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []models.Assignment
	for rows.Next() {
		a, x := scanA(rows)
		if x != nil {
			return nil, x
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

const ecols = `id,organization_id,settlement_organization_id,executing_provider_id,organization_membership_id,private_order_id,shipment_id,assignment_id,provider_response_id,vehicle_or_truck_reference,contract_reference,quotation_reference,execution_service,status,idempotency_key,request_fingerprint,canonical_request_reference,canonical_trip_reference,correlation_id,causation_id,execution_payload,source_version,attempts,last_error_code,next_attempt_at,lease_heartbeat_at,completed_at,dead_lettered_at,version,created_at,updated_at`
const ecolsClaimed = `o.id,o.organization_id,o.settlement_organization_id,o.executing_provider_id,o.organization_membership_id,o.private_order_id,o.shipment_id,o.assignment_id,o.provider_response_id,o.vehicle_or_truck_reference,o.contract_reference,o.quotation_reference,o.execution_service,o.status,o.idempotency_key,o.request_fingerprint,o.canonical_request_reference,o.canonical_trip_reference,o.correlation_id,o.causation_id,o.execution_payload,o.source_version,o.attempts,o.last_error_code,o.next_attempt_at,o.lease_heartbeat_at,o.completed_at,o.dead_lettered_at,o.version,o.created_at,o.updated_at`

func scanE(row pgx.Row) (models.ExecutionOperation, error) {
	var o models.ExecutionOperation
	var payload []byte
	e := row.Scan(&o.ID, &o.OrganizationID, &o.SettlementOrganizationID, &o.ExecutingProviderID, &o.OrganizationMembershipID, &o.PrivateOrderID, &o.ShipmentID, &o.AssignmentID, &o.ProviderResponseID, &o.VehicleOrTruckReference, &o.ContractReference, &o.QuotationReference, &o.ExecutionService, &o.Status, &o.IdempotencyKey, &o.RequestFingerprint, &o.CanonicalRequestReference, &o.CanonicalTripReference, &o.CorrelationID, &o.CausationID, &payload, &o.SourceVersion, &o.Attempts, &o.LastErrorCode, &o.NextAttemptAt, &o.LeaseHeartbeatAt, &o.CompletedAt, &o.DeadLetteredAt, &o.Version, &o.CreatedAt, &o.UpdatedAt)
	if errors.Is(e, pgx.ErrNoRows) {
		return o, ErrNotFound
	}
	if e == nil && len(payload) > 0 {
		e = json.Unmarshal(payload, &o.Payload)
	}
	return o, e
}
func (r *Repository) CreateExecution(ctx context.Context, q database.Querier, o models.ExecutionOperation) (models.ExecutionOperation, error) {
	payload, marshalErr := json.Marshal(o.Payload)
	if marshalErr != nil {
		return models.ExecutionOperation{}, marshalErr
	}
	v, e := scanE(r.q(q).QueryRow(ctx, `INSERT INTO fleet_partner_private_execution_operations(id,organization_id,settlement_organization_id,executing_provider_id,organization_membership_id,private_order_id,shipment_id,assignment_id,provider_response_id,vehicle_or_truck_reference,contract_reference,quotation_reference,execution_service,status,idempotency_key,request_fingerprint,correlation_id,causation_id,execution_payload)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19)RETURNING `+ecols, o.ID, o.OrganizationID, o.SettlementOrganizationID, o.ExecutingProviderID, o.OrganizationMembershipID, o.PrivateOrderID, o.ShipmentID, o.AssignmentID, o.ProviderResponseID, o.VehicleOrTruckReference, o.ContractReference, o.QuotationReference, o.ExecutionService, o.Status, o.IdempotencyKey, o.RequestFingerprint, o.CorrelationID, o.CausationID, payload))
	if e != nil {
		var pe *pgconn.PgError
		if errors.As(e, &pe) && pe.Code == "23505" {
			return v, ErrConflict
		}
	}
	return v, e
}
func (r *Repository) GetExecution(ctx context.Context, q database.Querier, org, id string) (models.ExecutionOperation, error) {
	return scanE(r.q(q).QueryRow(ctx, `SELECT `+ecols+` FROM fleet_partner_private_execution_operations WHERE organization_id=$1 AND id=$2`, org, id))
}

func (r *Repository) GetExecutionByID(ctx context.Context, q database.Querier, id string) (models.ExecutionOperation, error) {
	return scanE(r.q(q).QueryRow(ctx, `SELECT `+ecols+` FROM fleet_partner_private_execution_operations WHERE id=$1`, id))
}

func (r *Repository) GetExecutionByAssignment(ctx context.Context, q database.Querier, org, assignmentID string) (models.ExecutionOperation, error) {
	return scanE(r.q(q).QueryRow(ctx, `SELECT `+ecols+` FROM fleet_partner_private_execution_operations WHERE organization_id=$1 AND assignment_id=$2`, org, assignmentID))
}

func (r *Repository) RequestExecutionCancellation(ctx context.Context, q database.Querier, org, assignmentID string) error {
	tag, err := r.q(q).Exec(ctx, `UPDATE fleet_partner_private_execution_operations
		SET status='CANCELLATION_PENDING', next_attempt_at=now(), lease_owner=NULL,
			lease_expires_at=NULL, version=version+1, updated_at=now()
		WHERE organization_id=$1 AND assignment_id=$2
			AND status IN('PENDING','SUBMITTING','OUTCOME_UNKNOWN','CONFIRMED','RECONCILIATION_REQUIRED')`, org, assignmentID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}
func (r *Repository) ClaimExecutions(ctx context.Context, owner string, lease time.Duration, limit int) ([]models.ExecutionOperation, error) {
	rows, e := r.db.Query(ctx, `WITH c AS(SELECT id FROM fleet_partner_private_execution_operations WHERE status IN('PENDING','OUTCOME_UNKNOWN','CANCELLATION_PENDING','CANCELLATION_UNKNOWN','RECONCILIATION_REQUIRED')AND next_attempt_at<=now()AND(lease_expires_at IS NULL OR lease_expires_at<now())ORDER BY next_attempt_at FOR UPDATE SKIP LOCKED LIMIT $1)UPDATE fleet_partner_private_execution_operations o SET lease_owner=$2,lease_expires_at=now()+$3::interval,lease_heartbeat_at=now(),attempts=attempts+1,status=CASE WHEN status='PENDING' THEN 'SUBMITTING' ELSE status END FROM c WHERE o.id=c.id RETURNING `+ecolsClaimed, limit, owner, lease.String())
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []models.ExecutionOperation
	for rows.Next() {
		o, x := scanE(rows)
		if x != nil {
			return nil, x
		}
		out = append(out, o)
	}
	return out, rows.Err()
}
func (r *Repository) ResolveExecution(ctx context.Context, q database.Querier, owner, id, status, request, trip, code string, sourceVersion int64) error {
	tag, e := r.q(q).Exec(ctx, `UPDATE fleet_partner_private_execution_operations SET status=$1,canonical_request_reference=COALESCE(NULLIF($2,''),canonical_request_reference),canonical_trip_reference=COALESCE(NULLIF($3,''),canonical_trip_reference),last_error_code=$4,source_version=GREATEST(source_version,$5),lease_owner=NULL,lease_expires_at=NULL,lease_heartbeat_at=NULL,completed_at=CASE WHEN $1 IN('CONFIRMED','CANCELLED','FAILED') THEN now() ELSE completed_at END,version=version+1,updated_at=now() WHERE id=$6 AND lease_owner=$7`, status, request, trip, code, sourceVersion, id, owner)
	if e == nil && tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return e
}
func (r *Repository) RetryExecution(ctx context.Context, owner, id, status, code string, next time.Time) error {
	tag, e := r.db.Exec(ctx, `UPDATE fleet_partner_private_execution_operations SET status=$1,last_error_code=$2,next_attempt_at=$3,lease_owner=NULL,lease_expires_at=NULL,lease_heartbeat_at=NULL,version=version+1,updated_at=now() WHERE id=$4 AND lease_owner=$5`, status, code, next, id, owner)
	if e == nil && tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return e
}
func (r *Repository) Attempt(ctx context.Context, q database.Querier, org, id, outcome, code string, n int) error {
	_, e := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_private_execution_attempts(id,organization_id,operation_id,attempt,outcome,safe_error_code)VALUES($1,$2,$3,$4,$5,$6)`, ids.New(PrefixAttempt), org, id, n, outcome, code)
	return e
}

func (r *Repository) HeartbeatExecution(ctx context.Context, owner, id string, lease time.Duration) error {
	tag, err := r.db.Exec(ctx, `UPDATE fleet_partner_private_execution_operations
		SET lease_expires_at=now()+$3::interval,lease_heartbeat_at=now(),updated_at=now()
		WHERE id=$1 AND lease_owner=$2 AND lease_expires_at>now()`, id, owner, lease.String())
	if err == nil && tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return err
}

func (r *Repository) RecoverExpiredExecutions(ctx context.Context) (int64, error) {
	tag, err := r.db.Exec(ctx, `UPDATE fleet_partner_private_execution_operations SET
		status=CASE WHEN status='SUBMITTING' THEN 'OUTCOME_UNKNOWN' ELSE status END,
		last_error_code='lease_expired',next_attempt_at=now(),lease_owner=NULL,
		lease_expires_at=NULL,lease_heartbeat_at=NULL,version=version+1,updated_at=now()
		WHERE lease_owner IS NOT NULL AND lease_expires_at<now()
		AND status IN('SUBMITTING','OUTCOME_UNKNOWN','CANCELLATION_PENDING','CANCELLATION_UNKNOWN','RECONCILIATION_REQUIRED')`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (r *Repository) DeadLetterExecution(ctx context.Context, owner, id, code string) error {
	var updated bool
	err := r.db.QueryRow(ctx, `WITH dead AS(UPDATE fleet_partner_private_execution_operations SET
		status='DEAD_LETTER',last_error_code=$3,dead_lettered_at=now(),lease_owner=NULL,
		lease_expires_at=NULL,lease_heartbeat_at=NULL,version=version+1,updated_at=now()
		WHERE id=$1 AND lease_owner=$2 RETURNING id,organization_id,correlation_id),
		opened AS(INSERT INTO fleet_partner_private_execution_reconciliation_cases(
		id,organization_id,execution_operation_id,reason_code,correlation_id)
		SELECT $4,organization_id,id,'EXECUTION_DEAD_LETTER',correlation_id FROM dead
		ON CONFLICT DO NOTHING RETURNING id)
		SELECT EXISTS(SELECT 1 FROM dead)`, id, owner, code, ids.New("pxr")).Scan(&updated)
	if err == nil && !updated {
		return ErrConflict
	}
	return err
}

func (r *Repository) ReplayExecution(ctx context.Context, q database.Querier, org, id string) error {
	tag, err := r.q(q).Exec(ctx, `UPDATE fleet_partner_private_execution_operations SET
		status='PENDING',attempts=0,last_error_code='',next_attempt_at=now(),
		dead_lettered_at=NULL,version=version+1,updated_at=now()
		WHERE organization_id=$1 AND id=$2 AND status='DEAD_LETTER'
		AND canonical_trip_reference='' AND last_error_code NOT IN(
		'private_execution_outcome_unknown','lease_expired')`, org, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return err
}

func (r *Repository) RecoverExpiredReconciliations(ctx context.Context) (int64, error) {
	tag, err := r.db.Exec(ctx, `UPDATE fleet_partner_private_execution_reconciliation_cases SET
		status='RETRYABLE_FAILURE',last_error_code='lease_expired',next_attempt_at=now(),
		lease_owner=NULL,lease_expires_at=NULL,lease_heartbeat_at=NULL,updated_at=now()
		WHERE status='PROCESSING' AND lease_expires_at<now()`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (r *Repository) ClaimReconciliations(ctx context.Context, owner string, lease time.Duration,
	limit int) ([]models.ReconciliationCase, error) {
	rows, err := r.db.Query(ctx, `WITH claimed AS(
		SELECT id FROM fleet_partner_private_execution_reconciliation_cases
		WHERE status IN('PENDING','RETRYABLE_FAILURE') AND next_attempt_at<=now()
		AND(lease_expires_at IS NULL OR lease_expires_at<now())
		ORDER BY next_attempt_at,id FOR UPDATE SKIP LOCKED LIMIT $1)
		UPDATE fleet_partner_private_execution_reconciliation_cases c SET
		status='PROCESSING',attempts=attempts+1,lease_owner=$2,
		lease_expires_at=now()+$3::interval,lease_heartbeat_at=now(),updated_at=now()
		FROM claimed WHERE c.id=claimed.id RETURNING c.id,c.organization_id,
		c.execution_operation_id,COALESCE(c.projection_id,''),c.reason_code,c.status,
		c.attempts,c.correlation_id`, limit, owner, lease.String())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var cases []models.ReconciliationCase
	for rows.Next() {
		var item models.ReconciliationCase
		if err := rows.Scan(&item.ID, &item.OrganizationID, &item.ExecutionOperationID,
			&item.ProjectionID, &item.ReasonCode, &item.Status, &item.Attempts,
			&item.CorrelationID); err != nil {
			return nil, err
		}
		cases = append(cases, item)
	}
	return cases, rows.Err()
}

func (r *Repository) HeartbeatReconciliation(ctx context.Context, owner, id string, lease time.Duration) error {
	tag, err := r.db.Exec(ctx, `UPDATE fleet_partner_private_execution_reconciliation_cases SET
		lease_expires_at=now()+$3::interval,lease_heartbeat_at=now(),updated_at=now()
		WHERE id=$1 AND lease_owner=$2 AND status='PROCESSING' AND lease_expires_at>now()`,
		id, owner, lease.String())
	if err == nil && tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return err
}

func (r *Repository) ReconciliationAttempt(ctx context.Context, q database.Querier,
	item models.ReconciliationCase, outcome, code string) error {
	_, err := r.q(q).Exec(ctx, `INSERT INTO fleet_partner_private_execution_reconciliation_attempts(
		id,organization_id,reconciliation_case_id,execution_operation_id,attempt,outcome,safe_error_code)
		VALUES($1,$2,$3,$4,$5,$6,$7)`, ids.New("pra"), item.OrganizationID, item.ID,
		item.ExecutionOperationID, item.Attempts, outcome, code)
	return err
}

func (r *Repository) ResolveReconciliation(ctx context.Context, q database.Querier, owner, id string) error {
	tag, err := r.q(q).Exec(ctx, `UPDATE fleet_partner_private_execution_reconciliation_cases SET
		status='RESOLVED',last_error_code='',lease_owner=NULL,lease_expires_at=NULL,
		lease_heartbeat_at=NULL,updated_at=now() WHERE id=$1 AND lease_owner=$2 AND status='PROCESSING'`,
		id, owner)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return err
}

func (r *Repository) RetryReconciliation(ctx context.Context, owner, id, code string, next time.Time) error {
	tag, err := r.db.Exec(ctx, `UPDATE fleet_partner_private_execution_reconciliation_cases SET
		status='RETRYABLE_FAILURE',last_error_code=$3,next_attempt_at=$4,
		lease_owner=NULL,lease_expires_at=NULL,lease_heartbeat_at=NULL,updated_at=now()
		WHERE id=$1 AND lease_owner=$2 AND status='PROCESSING'`, id, owner, code, next)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return err
}

func (r *Repository) DeadLetterReconciliation(ctx context.Context, owner, id, code string) error {
	tag, err := r.db.Exec(ctx, `UPDATE fleet_partner_private_execution_reconciliation_cases SET
		status='DEAD_LETTER',last_error_code=$3,lease_owner=NULL,lease_expires_at=NULL,
		lease_heartbeat_at=NULL,updated_at=now() WHERE id=$1 AND lease_owner=$2 AND status='PROCESSING'`,
		id, owner, code)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return err
}

func (r *Repository) ReplayReconciliation(ctx context.Context, q database.Querier, org, id string) error {
	tag, err := r.q(q).Exec(ctx, `UPDATE fleet_partner_private_execution_reconciliation_cases SET
		status='PENDING',next_attempt_at=now(),last_error_code='',updated_at=now()
		WHERE organization_id=$1 AND id=$2 AND status='DEAD_LETTER'`, org, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return err
}

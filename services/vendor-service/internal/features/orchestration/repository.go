package orchestration

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/integrations/logistics"
)

// Repository owns the orchestration aggregate's statements.
type Repository struct{ db *pgxpool.Pool }

// NewRepository builds the repository.
func NewRepository(db *pgxpool.Pool) *Repository { return &Repository{db: db} }

// Pool exposes the pool for callers that own the transaction.
func (r *Repository) Pool() *pgxpool.Pool { return r.db }

const orchestrationColumns = `id, fulfilment_group_ref, parent_order_id, plan_version, state,
	requirements_snapshot, requirements_frozen_at, selected_authority, selected_product_code,
	capability_policy_version, selection_source, desired_pickup_start, desired_pickup_end,
	commit_not_before, next_evaluation_at, current_mission_ref, recovery_state,
	last_reason_code, version, created_at, updated_at`

func scanOrchestration(row pgx.Row) (Orchestration, error) {
	var o Orchestration
	var requirement []byte
	err := row.Scan(&o.ID, &o.FulfilmentGroupRef, &o.ParentOrderID, &o.PlanVersion, &o.State,
		&requirement, &o.RequirementsFrozenAt, &o.SelectedAuthority, &o.SelectedProductCode,
		&o.CapabilityPolicyVersion, &o.SelectionSource, &o.DesiredPickupStart, &o.DesiredPickupEnd,
		&o.CommitNotBefore, &o.NextEvaluationAt, &o.CurrentMissionRef, &o.RecoveryState,
		&o.LastReasonCode, &o.Version, &o.CreatedAt, &o.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return o, ErrNotFound
	}
	if err != nil {
		return o, err
	}
	_ = json.Unmarshal(requirement, &o.Requirement)
	return o, nil
}

// Open creates the orchestration for a fulfilment group, or returns the one
// that already exists.
//
// It is idempotent on the group reference because planning runs more than once
// -- on activation, on replan, and after a crash -- and a second row would mean
// two planners each believing they will collect the same goods.
func (r *Repository) Open(ctx context.Context, tx pgx.Tx, o Orchestration) (Orchestration, error) {
	requirement, err := json.Marshal(o.Requirement)
	if err != nil {
		return Orchestration{}, err
	}
	opened, err := scanOrchestration(tx.QueryRow(ctx, `
		INSERT INTO vendor_fulfilment_orchestrations(
			id, fulfilment_group_ref, parent_order_id, plan_version, state,
			requirements_snapshot, desired_pickup_start, desired_pickup_end,
			next_evaluation_at, selection_source)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (fulfilment_group_ref) DO NOTHING
		RETURNING `+orchestrationColumns,
		o.ID, o.FulfilmentGroupRef, o.ParentOrderID, o.PlanVersion, o.State,
		requirement, o.DesiredPickupStart, o.DesiredPickupEnd, o.NextEvaluationAt,
		o.SelectionSource))
	if err == nil {
		return opened, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Orchestration{}, err
	}
	return scanOrchestration(tx.QueryRow(ctx,
		`SELECT `+orchestrationColumns+` FROM vendor_fulfilment_orchestrations
		  WHERE fulfilment_group_ref=$1`, o.FulfilmentGroupRef))
}

// Get reads one orchestration.
func (r *Repository) Get(ctx context.Context, id string) (Orchestration, error) {
	return scanOrchestration(r.db.QueryRow(ctx,
		`SELECT `+orchestrationColumns+` FROM vendor_fulfilment_orchestrations WHERE id=$1`, id))
}

// GetByGroup reads one orchestration by its fulfilment group.
func (r *Repository) GetByGroup(ctx context.Context, groupRef string) (Orchestration, error) {
	return scanOrchestration(r.db.QueryRow(ctx,
		`SELECT `+orchestrationColumns+` FROM vendor_fulfilment_orchestrations
		  WHERE fulfilment_group_ref=$1`, groupRef))
}

// Lock reads an orchestration inside a transaction and holds it.
func (r *Repository) Lock(ctx context.Context, tx pgx.Tx, id string) (Orchestration, error) {
	return scanOrchestration(tx.QueryRow(ctx,
		`SELECT `+orchestrationColumns+` FROM vendor_fulfilment_orchestrations
		  WHERE id=$1 FOR UPDATE`, id))
}

// Update is one guarded state change.
type Update struct {
	State                   State
	SelectedAuthority       logistics.Authority
	SelectedProductCode     string
	CapabilityPolicyVersion string
	SelectionSource         SelectionSource
	CommitNotBefore         *time.Time
	NextEvaluationAt        *time.Time
	CurrentMissionRef       string
	RecoveryState           RecoveryState
	ReasonCode              string
	// FreezeRequirements marks the snapshot immutable. It is set exactly once,
	// as the orchestration begins committing, because from that moment the
	// authority has been asked about a specific requirement and changing it
	// would mean executing something nobody was asked about.
	FreezeRequirements bool
}

// Apply writes a state change, guarding on version.
func (r *Repository) Apply(ctx context.Context, tx pgx.Tx, id string,
	expectedVersion int64, update Update, now time.Time) (Orchestration, error) {
	var frozenAt *time.Time
	if update.FreezeRequirements {
		frozenAt = &now
	}
	updated, err := scanOrchestration(tx.QueryRow(ctx, `
		UPDATE vendor_fulfilment_orchestrations
		   SET state=$3,
		       selected_authority=COALESCE(NULLIF($4,''), selected_authority),
		       selected_product_code=COALESCE(NULLIF($5,''), selected_product_code),
		       capability_policy_version=COALESCE(NULLIF($6,''), capability_policy_version),
		       selection_source=COALESCE(NULLIF($7,''), selection_source),
		       commit_not_before=COALESCE($8, commit_not_before),
		       next_evaluation_at=COALESCE($9, next_evaluation_at),
		       current_mission_ref=COALESCE(NULLIF($10,''), current_mission_ref),
		       recovery_state=$11,
		       last_reason_code=$12,
		       requirements_frozen_at=COALESCE(requirements_frozen_at, $13),
		       version=version+1, updated_at=now()
		 WHERE id=$1 AND version=$2
		RETURNING `+orchestrationColumns,
		id, expectedVersion, update.State, string(update.SelectedAuthority),
		update.SelectedProductCode, update.CapabilityPolicyVersion,
		string(update.SelectionSource), update.CommitNotBefore, update.NextEvaluationAt,
		update.CurrentMissionRef, string(update.RecoveryState), update.ReasonCode, frozenAt))
	if errors.Is(err, ErrNotFound) {
		return Orchestration{}, ErrVersion
	}
	return updated, err
}

// SetRequirement replaces the snapshot before it is frozen.
//
// The database refuses this once requirements_frozen_at is set, so a replan
// after commitment fails loudly rather than silently changing what an authority
// was asked to do.
func (r *Repository) SetRequirement(ctx context.Context, tx pgx.Tx, id string,
	expectedVersion int64, requirement Requirement) (Orchestration, error) {
	raw, err := json.Marshal(requirement)
	if err != nil {
		return Orchestration{}, err
	}
	updated, err := scanOrchestration(tx.QueryRow(ctx, `
		UPDATE vendor_fulfilment_orchestrations
		   SET requirements_snapshot=$3, plan_version=plan_version+1,
		       version=version+1, updated_at=now()
		 WHERE id=$1 AND version=$2
		RETURNING `+orchestrationColumns, id, expectedVersion, raw))
	if errors.Is(err, ErrNotFound) {
		return Orchestration{}, ErrVersion
	}
	return updated, err
}

// ReplacePickups records which vendor orders this orchestration collects.
func (r *Repository) ReplacePickups(ctx context.Context, tx pgx.Tx, id string, pickups []Pickup) error {
	if _, err := tx.Exec(ctx,
		`DELETE FROM vendor_orchestration_pickups WHERE orchestration_id=$1`, id); err != nil {
		return err
	}
	for _, pickup := range pickups {
		handling := pickup.Handling
		if handling == nil {
			handling = []string{}
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO vendor_orchestration_pickups(
				orchestration_id, vendor_order_id, sequence, business_id, branch_id,
				pickup_latitude, pickup_longitude, weight_grams, volume_ml,
				package_count, handling, temperature_band, secure_pickup)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
			id, pickup.VendorOrderID, pickup.Sequence, pickup.BusinessID, pickup.BranchID,
			pickup.PickupLatitude, pickup.PickupLongitude, pickup.WeightGrams, pickup.VolumeMl,
			pickup.PackageCount, handling, pickup.TemperatureBand, pickup.SecurePickup); err != nil {
			if isUniqueViolation(err) {
				// Another live orchestration already holds this vendor order.
				// Two planners collecting the same goods is the one outcome
				// this index exists to prevent.
				return ErrVersion
			}
			return err
		}
	}
	return nil
}

type pickupQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

// Pickups returns the vendor orders this orchestration collects.
func (r *Repository) Pickups(ctx context.Context, id string) ([]Pickup, error) {
	return pickupsWith(ctx, r.db, id)
}

// PickupsTx returns the vendor orders using the caller's transaction. A caller
// that already holds a pooled connection must use this form: acquiring another
// connection while concurrent transactions hold the rest of a bounded pool can
// deadlock every participant.
func (r *Repository) PickupsTx(ctx context.Context, tx pgx.Tx, id string) ([]Pickup, error) {
	return pickupsWith(ctx, tx, id)
}

func pickupsWith(ctx context.Context, query pickupQuerier, id string) ([]Pickup, error) {
	rows, err := query.Query(ctx, `
		SELECT vendor_order_id, sequence, business_id, branch_id,
		       COALESCE(pickup_latitude,0), COALESCE(pickup_longitude,0),
		       weight_grams, volume_ml, package_count, handling,
		       temperature_band, secure_pickup, group_index, split_reason
		  FROM vendor_orchestration_pickups WHERE orchestration_id=$1 ORDER BY sequence`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Pickup{}
	for rows.Next() {
		var pickup Pickup
		if err = rows.Scan(&pickup.VendorOrderID, &pickup.Sequence,
			&pickup.BusinessID, &pickup.BranchID,
			&pickup.PickupLatitude, &pickup.PickupLongitude,
			&pickup.WeightGrams, &pickup.VolumeMl, &pickup.PackageCount,
			&pickup.Handling, &pickup.TemperatureBand, &pickup.SecurePickup,
			&pickup.GroupIndex, &pickup.SplitReason); err != nil {
			return nil, err
		}
		out = append(out, pickup)
	}
	return out, rows.Err()
}

// AssignGroups records the planner's output on each pickup.
//
// It writes group membership and, for a pickup that was split out, the specific
// incompatibility that split it. Both are written in the caller's transaction
// alongside the decision events that explain them, so a plan and its evidence
// are never half-recorded.
func (r *Repository) AssignGroups(ctx context.Context, tx pgx.Tx, id string,
	assignments map[string]GroupAssignment) error {
	for vendorOrderID, assignment := range assignments {
		if _, err := tx.Exec(ctx, `
			UPDATE vendor_orchestration_pickups
			   SET group_index=$3, split_reason=$4
			 WHERE orchestration_id=$1 AND vendor_order_id=$2`,
			id, vendorOrderID, assignment.GroupIndex, assignment.SplitReason); err != nil {
			return err
		}
	}
	return nil
}

// RecordDecision appends one piece of selection evidence.
func (r *Repository) RecordDecision(ctx context.Context, tx pgx.Tx, event DecisionEvent) error {
	var serviceable *bool
	if event.Serviceable != nil {
		serviceable = event.Serviceable
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO vendor_orchestration_events(
			id, orchestration_id, plan_version, event_type, requirement_snapshot,
			readiness_version, candidate_authority, candidate_product_code, serviceable,
			outcome_class, reason_code, supply_status, estimated_pickup_eta_seconds,
			policy_version, selected, actor_ref, override_reason, correlation_id)
		VALUES($1,$2,$3,$4,'{}'::jsonb,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
		"orce_"+uuid.NewString(), event.OrchestrationID, event.PlanVersion, event.EventType,
		event.ReadinessVersion, string(event.CandidateAuthority), event.CandidateProduct,
		serviceable, string(event.OutcomeClass), event.ReasonCode, event.SupplyStatus,
		event.EstimatedPickupETASeconds, event.PolicyVersion, event.Selected,
		event.ActorRef, event.OverrideReason, event.CorrelationID)
	return err
}

// Decisions returns one orchestration's evidence, newest first.
func (r *Repository) Decisions(ctx context.Context, id string, limit int) ([]DecisionEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, orchestration_id, plan_version, event_type, readiness_version,
		       candidate_authority, candidate_product_code, serviceable, outcome_class,
		       reason_code, supply_status, estimated_pickup_eta_seconds, policy_version,
		       selected, actor_ref, override_reason, correlation_id, created_at
		  FROM vendor_orchestration_events WHERE orchestration_id=$1
		 ORDER BY created_at DESC LIMIT $2`, id, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DecisionEvent{}
	for rows.Next() {
		var event DecisionEvent
		if err = rows.Scan(&event.ID, &event.OrchestrationID, &event.PlanVersion,
			&event.EventType, &event.ReadinessVersion, &event.CandidateAuthority,
			&event.CandidateProduct, &event.Serviceable, &event.OutcomeClass,
			&event.ReasonCode, &event.SupplyStatus, &event.EstimatedPickupETASeconds,
			&event.PolicyVersion, &event.Selected, &event.ActorRef, &event.OverrideReason,
			&event.CorrelationID, &event.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

// ClaimQuiet returns waiting orchestrations that have not been touched in a
// while, regardless of when their next evaluation is scheduled.
//
// It is the counterpart to ClaimDue, and the difference is the whole point. An
// orchestration whose kitchen has gone silent is not due for anything -- its
// next evaluation may be half an hour out, scheduled from a prediction that is
// now being made on a signal nobody has confirmed since. ClaimDue will never
// look at it. This does.
//
// It deliberately does NOT push next_evaluation_at out the way ClaimDue does.
// The caller re-evaluates what it claims, and that re-evaluation sets the
// timing from the widened prediction; moving it here first would make the
// sweep fight the planner for the same column.
func (r *Repository) ClaimQuiet(ctx context.Context, quietSince time.Time,
	limit int) ([]Orchestration, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
		SELECT `+orchestrationColumns+`
		  FROM vendor_fulfilment_orchestrations
		 WHERE state IN ('PLANNED','WAITING_READINESS','WARM_MATCH')
		   AND updated_at <= $1
		 ORDER BY updated_at
		 LIMIT $2 FOR UPDATE SKIP LOCKED`, quietSince, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectOrchestrations(rows)
}

// ClaimDue returns orchestrations whose next evaluation has arrived.
//
// SKIP LOCKED so several workers can run without contending, and without two
// of them evaluating the same orchestration into two different decisions.
func (r *Repository) ClaimDue(ctx context.Context, now time.Time, limit int) ([]Orchestration, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
		WITH due AS (
			SELECT id AS due_id FROM vendor_fulfilment_orchestrations
			 WHERE state IN ('PENDING_REQUIREMENTS','PLANNED','WAITING_READINESS','WARM_MATCH','COMMIT_DUE')
			   AND (next_evaluation_at IS NULL OR next_evaluation_at<=$1)
			 ORDER BY COALESCE(next_evaluation_at, created_at)
			 LIMIT $2 FOR UPDATE SKIP LOCKED)
		UPDATE vendor_fulfilment_orchestrations
		   SET next_evaluation_at=$1 + interval '30 seconds', updated_at=now()
		  FROM due WHERE vendor_fulfilment_orchestrations.id=due.due_id
		RETURNING `+orchestrationColumns, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectOrchestrations(rows)
}

// ClaimReconciling returns orchestrations whose mission outcome is unknown.
//
// These are the dangerous ones: a create was sent and nobody knows whether it
// landed. They are claimed separately from the due sweep so a reconciliation
// backlog cannot starve ordinary planning, and vice versa.
func (r *Repository) ClaimReconciling(ctx context.Context, now time.Time, limit int) ([]Orchestration, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
		WITH due AS (
			SELECT id AS due_id FROM vendor_fulfilment_orchestrations
			 WHERE recovery_state='RECONCILING'
			 ORDER BY updated_at LIMIT $2 FOR UPDATE SKIP LOCKED)
		UPDATE vendor_fulfilment_orchestrations
		   SET next_evaluation_at=$1 + interval '60 seconds', updated_at=now()
		  FROM due WHERE vendor_fulfilment_orchestrations.id=due.due_id
		RETURNING `+orchestrationColumns, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectOrchestrations(rows)
}

func collectOrchestrations(rows pgx.Rows) ([]Orchestration, error) {
	out := []Orchestration{}
	for rows.Next() {
		var o Orchestration
		var requirement []byte
		if err := rows.Scan(&o.ID, &o.FulfilmentGroupRef, &o.ParentOrderID, &o.PlanVersion,
			&o.State, &requirement, &o.RequirementsFrozenAt, &o.SelectedAuthority,
			&o.SelectedProductCode, &o.CapabilityPolicyVersion, &o.SelectionSource,
			&o.DesiredPickupStart, &o.DesiredPickupEnd, &o.CommitNotBefore,
			&o.NextEvaluationAt, &o.CurrentMissionRef, &o.RecoveryState, &o.LastReasonCode,
			&o.Version, &o.CreatedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(requirement, &o.Requirement)
		out = append(out, o)
	}
	return out, rows.Err()
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

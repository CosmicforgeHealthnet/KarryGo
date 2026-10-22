package deliveryrecovery

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Source-owned administrative interventions. Each function runs inside the
// caller's transaction (the one that also holds the immutable command ledger
// row) and drives the EXISTING recovery engines through their own scheduling
// fields and write paths. None of them fabricates an engine result, moves
// money, or transfers custody; every refusal leaves no side effect.

var ErrInterventionRejected = errors.New("delivery recovery intervention rejected by current state")
var ErrInterventionNotFound = errors.New("delivery recovery intervention subject not found")

// InterventionResult is what the owner state machine did. States and outcome
// are UPPER_SNAKE tokens; Effect holds durable before/after facts.
type InterventionResult struct {
	PriorState     string
	ResultingState string
	Outcome        string
	Gate           string
	EffectRef      string
	Effect         map[string]any
}

const (
	SubjectIncidents          = "incidents"
	SubjectMissions           = "missions"
	SubjectReturns            = "returns"
	SubjectDeliveryRecoveries = "delivery-recoveries"
)

func canonicalUUID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed != uuid.Nil && parsed.String() == id
}

func token(prefix, value string) string {
	v := strings.ToUpper(strings.TrimSpace(value))
	if v == "" {
		v = "UNKNOWN"
	}
	if prefix == "" {
		return v
	}
	return prefix + "_" + v
}

// InterventionState derives the subject's current owner state from the live
// domain rows the engines actually consult.
func InterventionState(ctx context.Context, tx pgx.Tx, kind, id string) (string, error) {
	if !canonicalUUID(id) {
		return "", ErrInterventionNotFound
	}
	var state string
	var err error
	switch kind {
	case SubjectIncidents:
		var incidentState, result string
		err = tx.QueryRow(ctx, `SELECT state,recovery_result FROM dispatch_delivery_incidents WHERE id=$1::uuid`, id).Scan(&incidentState, &result)
		state = incidentToken(incidentState, result)
	case SubjectMissions:
		var status string
		var result *string
		err = tx.QueryRow(ctx, `SELECT t.status,(SELECT i.recovery_result FROM dispatch_delivery_incidents i WHERE i.trip_id=t.id AND i.state='REASSIGNING' AND i.recovery_result<>'CANCELLED' ORDER BY i.created_at DESC LIMIT 1)
			FROM trips t WHERE t.id=$1::uuid`, id).Scan(&status, &result)
		state = token("TRIP", status)
		if result != nil {
			state = token("REASSIGNMENT", *result)
		}
	case SubjectReturns:
		var assignment string
		err = tx.QueryRow(ctx, `SELECT assignment_state FROM dispatch_return_workflows WHERE id=$1::uuid`, id).Scan(&assignment)
		state = token("ASSIGNMENT", assignment)
	case SubjectDeliveryRecoveries:
		var actionState string
		err = tx.QueryRow(ctx, `SELECT state FROM dispatch_delivery_recovery_actions WHERE id=$1::uuid`, id).Scan(&actionState)
		state = token("", actionState)
	default:
		return "", ErrInterventionNotFound
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrInterventionNotFound
	}
	return state, err
}

func incidentToken(state, result string) string {
	if state == "REASSIGNING" {
		return token("RECOVERY", result)
	}
	return token("INCIDENT", state)
}

type incidentRow struct {
	state, result, tripStatus string
	attempts                  int
	nextAt, deadline          time.Time
	leaseLive, sameProvider   bool
	missionReassigning        bool
}

// lockIncident locks only the incident row: every engine transition on an
// incident is a guarded UPDATE of that row, so it serializes them all.
func lockIncident(ctx context.Context, tx pgx.Tx, id string, now time.Time) (incidentRow, error) {
	var r incidentRow
	if !canonicalUUID(id) {
		return r, ErrInterventionNotFound
	}
	err := tx.QueryRow(ctx, `SELECT i.state,i.recovery_result,i.recovery_attempts,i.recovery_next_at,i.recovery_deadline,
		(i.recovery_lease_expires_at IS NOT NULL AND i.recovery_lease_expires_at>$2),t.status,t.provider_id=i.provider_id,
		EXISTS(SELECT 1 FROM dispatch_delivery_missions m JOIN dispatch_delivery_groups g ON g.id=m.delivery_group_id WHERE m.trip_id=t.id AND m.state='REASSIGNING' AND g.state NOT IN ('COMPLETED','CANCELLED','FAILED','DELIVERED','RETURNED'))
		FROM dispatch_delivery_incidents i JOIN trips t ON t.id=i.trip_id WHERE i.id=$1::uuid FOR UPDATE OF i`, id, now).
		Scan(&r.state, &r.result, &r.attempts, &r.nextAt, &r.deadline, &r.leaseLive, &r.tripStatus, &r.sameProvider, &r.missionReassigning)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrInterventionNotFound
	}
	return r, err
}

func terminalTrip(status string) bool {
	return status == "completed" || status == "cancelled" || status == "failed"
}

// RetryIncidentRecoveryTx brings the engine's own next attempt forward. It is
// allowed only where the existing claimer would itself retry: an incident in
// REASSIGNING with a PENDING/UNKNOWN result, no live worker lease, deadline
// and attempt budget remaining, and the Trip still held by the original
// provider with a reassigning mission. The engine decides the outcome.
func RetryIncidentRecoveryTx(ctx context.Context, tx pgx.Tx, id string, now time.Time, maxAttempts int) (InterventionResult, error) {
	r, err := lockIncident(ctx, tx, id, now)
	if err != nil {
		return InterventionResult{}, err
	}
	if r.state != "REASSIGNING" || (r.result != "PENDING" && r.result != "UNKNOWN") || r.leaseLive || !r.deadline.After(now) ||
		(maxAttempts > 0 && r.attempts >= maxAttempts) || terminalTrip(r.tripStatus) || !r.sameProvider || !r.missionReassigning {
		return InterventionResult{}, ErrInterventionRejected
	}
	if _, err = tx.Exec(ctx, `UPDATE dispatch_delivery_incidents SET recovery_next_at=$2,updated_at=$2 WHERE id=$1::uuid`, id, now); err != nil {
		return InterventionResult{}, err
	}
	prior := incidentToken(r.state, r.result)
	return InterventionResult{PriorState: prior, ResultingState: prior, Outcome: "RECOVERY_RETRY_SCHEDULED", EffectRef: id,
		Effect: map[string]any{"prior_next_attempt_at": r.nextAt.UTC(), "next_attempt_at": now.UTC(), "recovery_attempts": r.attempts, "recovery_deadline": r.deadline.UTC()}}, nil
}

// EscalateIncidentRecoveryTx moves an in-flight recovery to the engine's own
// operations-required state (MANUAL_REVIEW) and withdraws its open offers,
// exactly as the engine does on exhaustion. A READY replacement (already
// accepted) or a terminal result cannot be escalated.
func EscalateIncidentRecoveryTx(ctx context.Context, tx pgx.Tx, id, escalation string, now time.Time) (InterventionResult, error) {
	if escalation != "OPERATIONS_REVIEW" && escalation != "SAFETY_REVIEW" {
		return InterventionResult{}, ErrInterventionRejected
	}
	r, err := lockIncident(ctx, tx, id, now)
	if err != nil {
		return InterventionResult{}, err
	}
	if r.state != "REASSIGNING" || (r.result != "PENDING" && r.result != "UNKNOWN") {
		return InterventionResult{}, ErrInterventionRejected
	}
	code := "ADMIN_ESCALATED_" + escalation
	if _, err = tx.Exec(ctx, `UPDATE dispatch_delivery_incidents SET recovery_result='MANUAL_REVIEW',recovery_error_code=$2,updated_at=$3 WHERE id=$1::uuid`, id, code, now); err != nil {
		return InterventionResult{}, err
	}
	tag, err := tx.Exec(ctx, `UPDATE dispatch_assignment_offers SET status='CANCELLED',version=version+1 WHERE incident_id=$1::uuid AND status='OFFERED'`, id)
	if err != nil {
		return InterventionResult{}, err
	}
	return InterventionResult{PriorState: incidentToken(r.state, r.result), ResultingState: "RECOVERY_MANUAL_REVIEW", Outcome: "RECOVERY_ESCALATED", EffectRef: id,
		Effect: map[string]any{"escalation": escalation, "recovery_error_code": code, "withdrawn_offers": tag.RowsAffected(), "worker_lease_live": r.leaseLive}}, nil
}

var reassignmentReasons = map[string]string{
	"PROVIDER_UNRESPONSIVE": "OPERATIONS_DIRECTED_REPLACEMENT",
	"PROVIDER_UNSAFE":       "SAFETY_CONCERN",
	"VEHICLE_FAILURE":       "VEHICLE_FAILURE",
	"OPERATIONS_DIRECTED":   "OPERATIONS_DIRECTED_REPLACEMENT",
}

// RequestOperationsReplacementTx opens the existing incident replacement flow
// for a Trip that is still before custody: the same incident row, mission
// REASSIGNING transition and events a provider report produces, which the
// recovery worker then claims and offers through the marketplace.
//
// Refused (no side effect) when custody is held or may be held (Trip past
// arrival, pickup handover active/consumed/locked, mission past pickup, an
// active custody holding), when an uncertain provider-decision relay is open
// for the Trip (it must be read back first), when a replacement is already
// active, or when there is no dispatchable mission for the engine to work.
func RequestOperationsReplacementTx(ctx context.Context, tx pgx.Tx, tripID, reason, decisionID string, now time.Time) (InterventionResult, error) {
	incidentReason, ok := reassignmentReasons[reason]
	if !ok {
		return InterventionResult{}, ErrInterventionRejected
	}
	if !canonicalUUID(tripID) {
		return InterventionResult{}, ErrInterventionNotFound
	}
	trip, err := lockTrip(ctx, tx, tripID, "")
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return InterventionResult{}, ErrInterventionNotFound
		}
		return InterventionResult{}, err
	}
	prior := token("TRIP", trip.Status)
	if trip.Status != "assigned" && trip.Status != "en_route_pickup" && trip.Status != "arrived_pickup" {
		return InterventionResult{}, ErrInterventionRejected
	}
	// Lock the missions before reading custody facts: the pickup handover
	// locks the same mission row, so its commit is observed, never raced.
	rows, err := tx.Query(ctx, `SELECT m.state,g.state FROM dispatch_delivery_missions m JOIN dispatch_delivery_groups g ON g.id=m.delivery_group_id WHERE m.trip_id=$1::uuid ORDER BY m.id FOR UPDATE OF m`, tripID)
	if err != nil {
		return InterventionResult{}, err
	}
	dispatchable := 0
	unsafe := false
	for rows.Next() {
		var mission, group string
		if err = rows.Scan(&mission, &group); err != nil {
			rows.Close()
			return InterventionResult{}, err
		}
		switch mission {
		case "ASSIGNED", "EN_ROUTE_PICKUP", "AT_PICKUP":
			switch group {
			case "COMPLETED", "CANCELLED", "FAILED", "DELIVERED", "RETURNED":
			default:
				dispatchable++
			}
		case "DELIVERED", "COMPLETED", "CANCELLED", "RETURNED":
		default:
			unsafe = true
		}
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return InterventionResult{}, err
	}
	if unsafe || dispatchable == 0 {
		return InterventionResult{}, ErrInterventionRejected
	}
	var custodyHeld, handoverOpen, uncertain, activeIncident bool
	err = tx.QueryRow(ctx, `SELECT
		EXISTS(SELECT 1 FROM custody_manifest_holdings h JOIN commerce_missions cm ON h.mission_ref IN (cm.canonical_ref,cm.mission_ref) WHERE (cm.trip_id=$1::uuid OR cm.replacement_trip_id=$1::uuid) AND h.released_at IS NULL)
		OR EXISTS(SELECT 1 FROM custody_mission_state s JOIN commerce_missions cm ON s.mission_ref IN (cm.canonical_ref,cm.mission_ref) WHERE (cm.trip_id=$1::uuid OR cm.replacement_trip_id=$1::uuid) AND s.status<>'PICKUP_REQUIRED')
		OR EXISTS(SELECT 1 FROM dispatch_active_custody_manifest_parcels a JOIN dispatch_delivery_missions m ON m.id=a.mission_id WHERE m.trip_id=$1::uuid AND m.state NOT IN ('ASSIGNED','EN_ROUTE_PICKUP','AT_PICKUP','PLANNED','QUEUED','OFFERING')),
		EXISTS(SELECT 1 FROM dispatch_handover_challenges c WHERE c.trip_id=$1::uuid AND c.stage='PICKUP' AND c.status IN ('ACTIVE','CONSUMED','LOCKED')),
		EXISTS(SELECT 1 FROM dispatch_fleet_relay_operations o WHERE (o.readback_required OR o.status IN ('PENDING','DELIVERING','RETRY_SCHEDULED','UNKNOWN_OUTCOME'))
			AND ((t.fleet_assignment_id IS NOT NULL AND o.assignment_id=t.fleet_assignment_id) OR o.request_id IN (SELECT r.id FROM request_broadcasts r WHERE r.booking_id=t.booking_id))),
		EXISTS(SELECT 1 FROM dispatch_delivery_incidents i WHERE i.trip_id=$1::uuid AND i.state NOT IN ('RESOLVED','CANCELLED'))
		FROM trips t WHERE t.id=$1::uuid`, tripID).Scan(&custodyHeld, &handoverOpen, &uncertain, &activeIncident)
	if err != nil {
		return InterventionResult{}, err
	}
	if custodyHeld || handoverOpen || uncertain || activeIncident {
		return InterventionResult{}, ErrInterventionRejected
	}
	command := IncidentCommand{TripID: trip.TripID, ProviderID: trip.ProviderID, ReasonCode: incidentReason,
		PreferredStrategy: "SEQUENTIAL_NEAREST", IdempotencyKey: "admin-intervention:" + decisionID}
	incident, err := insertIncidentTx(ctx, tx, trip, command, now, fingerprint(struct{ Trip, Reason, Decision string }{tripID, reason, decisionID}))
	if err != nil {
		return InterventionResult{}, err
	}
	return InterventionResult{PriorState: prior, ResultingState: "REASSIGNMENT_PENDING", Outcome: "REPLACEMENT_REQUESTED", EffectRef: incident.ID,
		Effect: map[string]any{"incident_id": incident.ID, "incident_reason": incidentReason, "original_provider_id": trip.ProviderID, "reassigning_mission_id": incident.MissionID, "captured_trip_revision": incident.SourceVersion}}, nil
}

// RearmReturnAssignmentTx re-arms the existing return-execution assignment
// retry. A waiting retry (PENDING/UNKNOWN or an expired DELIVERING lease) is
// brought forward; a DEAD_LETTER (budget exhausted, last outcome unknown) is
// re-armed as UNKNOWN with a fresh budget so the worker reads back through
// the same idempotent operation reference. Refused when the return mission
// is already assigned or no longer dispatchable, its Wallet state is not
// cleared, a worker holds a live lease, or the assignment is acknowledged.
func RearmReturnAssignmentTx(ctx context.Context, tx pgx.Tx, id string, now time.Time, maxAttempts int) (InterventionResult, error) {
	if !canonicalUUID(id) {
		return InterventionResult{}, ErrInterventionNotFound
	}
	if maxAttempts <= 0 {
		maxAttempts = 10
	}
	var assignment, workflow, wallet string
	var attempts int
	var nextAt time.Time
	var leaseLive bool
	var missionState *string
	var unassigned *bool
	err := tx.QueryRow(ctx, `SELECT w.assignment_state,w.state,w.wallet_state,w.assignment_attempt_count,w.assignment_next_attempt_at,
		(w.assignment_lease_expires_at IS NOT NULL AND w.assignment_lease_expires_at>$2),m.state,m.trip_id IS NULL
		FROM dispatch_return_workflows w LEFT JOIN dispatch_delivery_missions m ON m.id=w.return_mission_id WHERE w.id=$1::uuid FOR UPDATE OF w`, id, now).
		Scan(&assignment, &workflow, &wallet, &attempts, &nextAt, &leaseLive, &missionState, &unassigned)
	if errors.Is(err, pgx.ErrNoRows) {
		return InterventionResult{}, ErrInterventionNotFound
	}
	if err != nil {
		return InterventionResult{}, err
	}
	if missionState == nil || unassigned == nil || !*unassigned || (*missionState != "PLANNED" && *missionState != "OFFERING" && *missionState != "QUEUED") ||
		(wallet != "NOT_REQUIRED" && wallet != "APPROVED" && wallet != "ACKNOWLEDGED") || leaseLive {
		return InterventionResult{}, ErrInterventionRejected
	}
	prior := token("ASSIGNMENT", assignment)
	effect := map[string]any{"prior_next_attempt_at": nextAt.UTC(), "next_attempt_at": now.UTC(), "prior_attempt_count": attempts, "prior_workflow_state": workflow}
	switch assignment {
	case "PENDING", "UNKNOWN", "DELIVERING":
		if (workflow != "REQUIRED" && workflow != "ROUTED") || attempts >= maxAttempts {
			return InterventionResult{}, ErrInterventionRejected
		}
		if _, err = tx.Exec(ctx, `UPDATE dispatch_return_workflows SET assignment_next_attempt_at=$2,updated_at=$2 WHERE id=$1::uuid`, id, now); err != nil {
			return InterventionResult{}, err
		}
		return InterventionResult{PriorState: prior, ResultingState: prior, Outcome: "ASSIGNMENT_RETRY_SCHEDULED", EffectRef: id, Effect: effect}, nil
	case "DEAD_LETTER":
		if workflow != "SUPPORT_REQUIRED" {
			return InterventionResult{}, ErrInterventionRejected
		}
		if _, err = tx.Exec(ctx, `UPDATE dispatch_return_workflows SET assignment_state='UNKNOWN',assignment_attempt_count=0,assignment_next_attempt_at=$2,
			assignment_lease_owner=NULL,assignment_lease_expires_at=NULL,state='ROUTED',updated_at=$2 WHERE id=$1::uuid AND assignment_state='DEAD_LETTER'`, id, now); err != nil {
			return InterventionResult{}, err
		}
		effect["workflow_state"] = "ROUTED"
		return InterventionResult{PriorState: prior, ResultingState: "ASSIGNMENT_UNKNOWN", Outcome: "ASSIGNMENT_RETRY_REARMED", EffectRef: id, Effect: effect}, nil
	}
	return InterventionResult{}, ErrInterventionRejected
}

var replanActions = map[string]RecoveryAction{"RESCHEDULE_ATTEMPT": Reschedule, "RETURN_TO_SENDER": ReturnToSender}

// ReplanRecoveryTx requests a different delivery recovery plan. Operations
// cannot choose a reschedule time, a return charge or a financial
// authorization for the customer, so the request never executes the plan:
// it parks the current action in AWAITING_CUSTOMER_APPROVAL and reopens the
// attempt for the customer with only the requested action allowed. The
// customer's existing selection path (schedule, route and financial gates)
// is the approval; it completes the parked action when it commits.
//
// Only a not-yet-executed plan without a financial authorization can be
// replanned: a SCHEDULED retry/reschedule, or an OPERATIONS_REVIEW.
func ReplanRecoveryTx(ctx context.Context, tx pgx.Tx, id, replan string, returnEnabled bool, now time.Time) (InterventionResult, error) {
	target, ok := replanActions[replan]
	if !ok {
		return InterventionResult{}, ErrInterventionRejected
	}
	if !canonicalUUID(id) {
		return InterventionResult{}, ErrInterventionNotFound
	}
	var tripID string
	err := tx.QueryRow(ctx, `SELECT trip_id::text FROM dispatch_delivery_recovery_actions WHERE id=$1::uuid`, id).Scan(&tripID)
	if errors.Is(err, pgx.ErrNoRows) {
		return InterventionResult{}, ErrInterventionNotFound
	}
	if err != nil {
		return InterventionResult{}, err
	}
	// Same lock order as customer selection and scheduled activation.
	trip, err := lockTrip(ctx, tx, tripID, "")
	if err != nil {
		return InterventionResult{}, err
	}
	var state, action, financial, attemptID string
	if err = tx.QueryRow(ctx, `SELECT state,action,financial_state,delivery_attempt_id::text FROM dispatch_delivery_recovery_actions WHERE id=$1::uuid FOR UPDATE`, id).
		Scan(&state, &action, &financial, &attemptID); err != nil {
		return InterventionResult{}, err
	}
	var attemptState, reason string
	var attemptVersion int64
	if err = tx.QueryRow(ctx, `SELECT state,failure_reason,source_version FROM dispatch_delivery_attempts WHERE id=$1::uuid FOR UPDATE`, attemptID).
		Scan(&attemptState, &reason, &attemptVersion); err != nil {
		return InterventionResult{}, err
	}
	eligible := (state == "SCHEDULED" && (action == string(RetrySameDay) || action == string(Reschedule))) || (state == "READY" && action == string(OperationsReview))
	if !eligible || financial != "NOT_REQUIRED" || attemptState != string(AttemptRecoverySelected) || terminalTrip(trip.Status) ||
		!containsAction(receiverReasons[reason], target) || (target == ReturnToSender && !returnEnabled) {
		return InterventionResult{}, ErrInterventionRejected
	}
	if _, err = tx.Exec(ctx, `UPDATE dispatch_delivery_recovery_actions SET state='AWAITING_CUSTOMER_APPROVAL',updated_at=$2 WHERE id=$1::uuid`, id, now); err != nil {
		return InterventionResult{}, err
	}
	allowed := []RecoveryAction{target}
	raw, _ := json.Marshal(allowed)
	if _, err = tx.Exec(ctx, `UPDATE dispatch_delivery_attempts SET state='RECEIVER_UNAVAILABLE',allowed_actions=$2,source_version=$3,updated_at=$4 WHERE id=$1::uuid`,
		attemptID, raw, attemptVersion+1, now); err != nil {
		return InterventionResult{}, err
	}
	// The existing customer prompt to choose a recovery for this attempt.
	if err = emit(ctx, tx, "dispatch.receiver_unavailable.recorded.v1", "trip", trip.TripID, attemptVersion+1,
		map[string]any{"attempt_id": attemptID, "trip_id": trip.TripID, "state": AttemptReceiverUnavailable, "allowed_actions": allowed, "source_version": attemptVersion + 1}); err != nil {
		return InterventionResult{}, err
	}
	return InterventionResult{PriorState: token("", state), ResultingState: "AWAITING_CUSTOMER_APPROVAL", Outcome: "REPLAN_AWAITING_CUSTOMER_APPROVAL", Gate: "AWAITING_CUSTOMER_APPROVAL", EffectRef: attemptID,
		Effect: map[string]any{"prior_action": action, "requested_action": target, "delivery_attempt_id": attemptID, "failure_reason": reason}}, nil
}

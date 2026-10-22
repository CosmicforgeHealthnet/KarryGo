// Package adminintervention implements Dispatch's closed administrative
// command dictionary (shared/go/admincommand) as case-bound interventions on
// the existing incident-recovery, provider-replacement, return-execution and
// delivery-recovery state machines. It never sets a domain state directly:
// each action is applied through deliveryrecovery's own engine write paths,
// guarded by a per-subject intervention head (the version fence) and recorded
// in an append-only intervention record bound to the ledger decision.
package adminintervention

import (
	"context"
	"encoding/json"
	"errors"

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/config"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/adminresources"
	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/deliveryrecovery"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/adminowner"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Prefix names this owner's command environment switches.
const Prefix = "DISPATCH"

// Engines reports which existing engines will actually act on an
// intervention in this deployment. An intervention that no running engine
// would pick up is refused rather than recorded as if it would happen.
type Engines struct {
	// IncidentRecovery: the recovery worker claims REASSIGNING incidents
	// (delivery recovery + replacement handover + an assignment authority).
	IncidentRecovery bool
	// ReturnAssignment: the recovery worker dispatches return missions.
	ReturnAssignment bool
	// CustomerRecovery: customers can select a recovery (the replan gate).
	CustomerRecovery bool
	// ReturnToSender: the return workflow is enabled.
	ReturnToSender bool
	// MaxAttempts is the worker's own retry budget.
	MaxAttempts int
}

func EnginesFromConfig(cfg config.Config) Engines {
	assignment := (cfg.SequentialAssignmentEnabled || cfg.OrganizationAssignmentEnabled || cfg.AdminAssistedAssignmentEnabled) && !cfg.AssignmentKillSwitch
	return Engines{
		IncidentRecovery: cfg.DeliveryRecoveryEnabled && cfg.ReplacementHandoverEnabled && assignment,
		ReturnAssignment: cfg.DeliveryRecoveryEnabled && assignment,
		CustomerRecovery: cfg.DeliveryRecoveryEnabled,
		ReturnToSender:   cfg.DeliveryRecoveryEnabled && cfg.ReturnWorkflowEnabled,
		MaxAttempts:      cfg.ExternalOperationRetryLimit,
	}
}

type Executor struct{ Engines Engines }

var _ admincommand.Executor = (*Executor)(nil)

func lockHead(ctx context.Context, tx pgx.Tx, kind, id string) (int64, error) {
	var version int64
	err := tx.QueryRow(ctx, `SELECT version FROM dispatch_admin_intervention_heads WHERE subject_kind=$1 AND subject_id=$2 FOR UPDATE`, kind, id).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	return version, err
}

func domainError(err error) error {
	switch {
	case errors.Is(err, deliveryrecovery.ErrInterventionRejected):
		return admincommand.ErrRejected
	case errors.Is(err, deliveryrecovery.ErrInterventionNotFound):
		return admincommand.ErrNotFound
	}
	return err
}

func (e *Executor) apply(ctx context.Context, tx pgx.Tx, in admincommand.Execution) (deliveryrecovery.InterventionResult, error) {
	maxAttempts := e.Engines.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 10
	}
	switch {
	case in.Action == "RETRY_INCIDENT_RECOVERY" && in.SubjectKind == deliveryrecovery.SubjectIncidents:
		if !e.Engines.IncidentRecovery {
			return deliveryrecovery.InterventionResult{}, deliveryrecovery.ErrInterventionRejected
		}
		return deliveryrecovery.RetryIncidentRecoveryTx(ctx, tx, in.SubjectID, in.Now, maxAttempts)
	case in.Action == "ESCALATE_INCIDENT_RECOVERY" && in.SubjectKind == deliveryrecovery.SubjectIncidents:
		return deliveryrecovery.EscalateIncidentRecoveryTx(ctx, tx, in.SubjectID, in.Params["escalation"], in.Now)
	case in.Action == "REQUEST_REASSIGNMENT" && in.SubjectKind == deliveryrecovery.SubjectMissions:
		if !e.Engines.IncidentRecovery {
			return deliveryrecovery.InterventionResult{}, deliveryrecovery.ErrInterventionRejected
		}
		return deliveryrecovery.RequestOperationsReplacementTx(ctx, tx, in.SubjectID, in.Params["reason"], in.DecisionID, in.Now)
	case in.Action == "RETRY_RETURN_ASSIGNMENT" && in.SubjectKind == deliveryrecovery.SubjectReturns:
		if !e.Engines.ReturnAssignment {
			return deliveryrecovery.InterventionResult{}, deliveryrecovery.ErrInterventionRejected
		}
		return deliveryrecovery.RearmReturnAssignmentTx(ctx, tx, in.SubjectID, in.Now, maxAttempts)
	case in.Action == "REQUEST_DELIVERY_REPLAN" && in.SubjectKind == deliveryrecovery.SubjectDeliveryRecoveries:
		if !e.Engines.CustomerRecovery {
			return deliveryrecovery.InterventionResult{}, deliveryrecovery.ErrInterventionRejected
		}
		return deliveryrecovery.ReplanRecoveryTx(ctx, tx, in.SubjectID, in.Params["replan"], e.Engines.ReturnToSender, in.Now)
	}
	return deliveryrecovery.InterventionResult{}, deliveryrecovery.ErrInterventionRejected
}

// Apply fences the subject's intervention head, applies the action through
// the owning engine, then advances the head and appends the record. Every
// step shares the caller's transaction with the immutable ledger row.
func (e *Executor) Apply(ctx context.Context, tx pgx.Tx, in admincommand.Execution) (admincommand.Transition, error) {
	version, err := lockHead(ctx, tx, in.SubjectKind, in.SubjectID)
	if err != nil {
		return admincommand.Transition{}, err
	}
	if _, err = deliveryrecovery.InterventionState(ctx, tx, in.SubjectKind, in.SubjectID); err != nil {
		return admincommand.Transition{}, domainError(err)
	}
	if version != in.ExpectedVersion {
		return admincommand.Transition{}, admincommand.ErrConflict
	}
	result, err := e.apply(ctx, tx, in)
	if err != nil {
		return admincommand.Transition{}, domainError(err)
	}
	next := version + 1
	if version == 0 {
		_, err = tx.Exec(ctx, `INSERT INTO dispatch_admin_intervention_heads(subject_kind,subject_id,version,state,last_action,last_decision_id,created_at,updated_at) VALUES($1,$2,1,$3,$4,$5,$6,$6)`,
			in.SubjectKind, in.SubjectID, result.ResultingState, in.Action, in.DecisionID, in.Now)
	} else {
		_, err = tx.Exec(ctx, `UPDATE dispatch_admin_intervention_heads SET version=$3,state=$4,last_action=$5,last_decision_id=$6,updated_at=$7 WHERE subject_kind=$1 AND subject_id=$2 AND version=$8`,
			in.SubjectKind, in.SubjectID, next, result.ResultingState, in.Action, in.DecisionID, in.Now, version)
	}
	if err != nil {
		return admincommand.Transition{}, conflict(err)
	}
	params, _ := json.Marshal(in.Params)
	effect, err := json.Marshal(result.Effect)
	if err != nil || result.Effect == nil {
		effect = []byte(`{}`)
	}
	gate := result.Gate
	if gate == "" {
		gate = "NONE"
	}
	if _, err = tx.Exec(ctx, `INSERT INTO dispatch_admin_intervention_records(decision_id,subject_kind,subject_id,action,recorded_version,prior_state,resulting_state,outcome,gate,reason_code,params,effect_ref,effect,recorded_at)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, in.DecisionID, in.SubjectKind, in.SubjectID, in.Action, next, result.PriorState, result.ResultingState,
		result.Outcome, gate, in.ReasonCode, params, result.EffectRef, effect, in.Now); err != nil {
		return admincommand.Transition{}, conflict(err)
	}
	return admincommand.Transition{PriorVersion: version, PriorState: result.PriorState, ResultingState: result.ResultingState, Outcome: result.Outcome}, nil
}

func conflict(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return admincommand.ErrConflict
	}
	return err
}

// Current is the subject's intervention version and its live owner state as
// the engines see it now (used by history and post-ambiguity readback).
func (e *Executor) Current(ctx context.Context, tx pgx.Tx, kind, id string) (int64, string, error) {
	version, err := func() (int64, error) {
		var v int64
		err := tx.QueryRow(ctx, `SELECT version FROM dispatch_admin_intervention_heads WHERE subject_kind=$1 AND subject_id=$2`, kind, id).Scan(&v)
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return v, err
	}()
	if err != nil {
		return 0, "", err
	}
	state, err := deliveryrecovery.InterventionState(ctx, tx, kind, id)
	if err != nil {
		return 0, "", domainError(err)
	}
	return version, state, nil
}

// Runtime composes the signed command boundary from the already initialized
// Dispatch owner handler. Both switches default off.
func Runtime(owner *adminowner.Handler, engines Engines) (*admincommand.Handler, error) {
	spec, ok := admincommand.ForOwner(adminresources.Owner)
	if !ok {
		return nil, admincommand.ErrConfiguration
	}
	return admincommand.Runtime(owner, spec, &Executor{Engines: engines}, Prefix)
}

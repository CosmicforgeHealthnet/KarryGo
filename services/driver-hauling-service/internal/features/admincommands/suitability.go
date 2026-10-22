package admincommands

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"cosmicforge/logistics/services/hauling-service/internal/cargofit"
	"cosmicforge/logistics/services/hauling-service/internal/features/recovery"
	"cosmicforge/logistics/shared/go/admincommand"
	"github.com/jackc/pgx/v5"
)

// Cargo suitability (VersionOf SUBJECT, subject = booking-cargo-inputs):
//
//	SUITABLE                  only when the truck CURRENTLY fits the booking's
//	                          CURRENT inputs; it never authorizes assignment.
//	UNSUITABLE                blocks that truck for that booking.
//	SUITABLE_WITH_REMEDIATION records a PENDING remediation that blocks the
//	                          truck until COMPLETE_CARGO_REMEDIATION, which
//	                          itself requires the truck to currently fit.
const (
	decisionSuitable    = "SUITABLE"
	decisionUnsuitable  = "UNSUITABLE"
	decisionRemediation = "SUITABLE_WITH_REMEDIATION"
)

func suitabilityOpen(status string) bool {
	switch status {
	case "completed", "cancelled", "unmatched", "delivered":
		return false
	}
	return true
}

type fitResult struct {
	eligible    bool
	reason      string
	fingerprint string
	capability  int64
}

// currentFit evaluates the truck against the booking's CURRENT inputs with the
// existing cargofit requirement derivation and truck capability store.
func (e *Executor) currentFit(ctx context.Context, tx pgx.Tx, b bookingRow, truckID string) (fitResult, error) {
	var capability int64
	if err := tx.QueryRow(ctx, `SELECT capability_version FROM trucks WHERE id::text=$1`, truckID).Scan(&capability); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fitResult{}, admincommand.ErrRejected
		}
		return fitResult{}, err
	}
	if e.Fit == nil {
		return fitResult{}, admincommand.ErrRejected
	}
	requirement := recovery.RequirementFor(recovery.BookingSnapshot{CargoWeightKg: b.weightKg, PreferredTruckType: b.preferredType, IsFragile: b.fragile})
	requirement.ProductCode, requirement.CertificationRequired = e.Fit.ProductForRequirement(requirement)
	decision := e.Fit.EvaluateTruck(ctx, truckID, requirement)
	raw, _ := json.Marshal(struct {
		Requirement cargofit.Requirement
		Truck       string
		Capability  int64
	}{requirement, truckID, capability})
	sum := sha256.Sum256(raw)
	out := fitResult{fingerprint: hex.EncodeToString(sum[:]), capability: capability}
	if decision != nil {
		out.eligible, out.reason = decision.Eligible, decision.PrimaryReason()
	}
	return out, nil
}

func (e *Executor) recordSuitability(ctx context.Context, tx pgx.Tx, in admincommand.Execution) (admincommand.Transition, error) {
	b, err := lockBooking(ctx, tx, in.SubjectID, true)
	if err != nil {
		return admincommand.Transition{}, err
	}
	if err := requireOptionalTenant(in, b.organization); err != nil {
		return admincommand.Transition{}, err
	}
	prior, err := fence(ctx, tx, in)
	if err != nil {
		return admincommand.Transition{}, err
	}
	if !suitabilityOpen(b.status) {
		return admincommand.Transition{}, admincommand.ErrRejected
	}
	truckID, decision := in.Params["truck_id"], in.Params["decision"]
	fit, err := e.currentFit(ctx, tx, b, truckID)
	if err != nil {
		return admincommand.Transition{}, err
	}
	var pendingTruck string
	err = tx.QueryRow(ctx, `SELECT truck_id::text FROM hauling_cargo_suitability_decisions WHERE booking_id::text=$1 AND remediation_state='PENDING' FOR UPDATE`, b.id).Scan(&pendingTruck)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return admincommand.Transition{}, err
	}
	remediation, resulting, outcome := "NONE", decision, ""
	switch decision {
	case decisionSuitable:
		// Historical fit never authorizes: SUITABLE is accepted only on a
		// CURRENT pass, and never replaces a remediation still owed.
		if !fit.eligible || pendingTruck == truckID {
			return admincommand.Transition{}, admincommand.ErrRejected
		}
		outcome = "CARGO_SUITABLE_RECORDED"
	case decisionUnsuitable:
		outcome = "CARGO_UNSUITABLE_RECORDED"
	case decisionRemediation:
		if pendingTruck != "" {
			return admincommand.Transition{}, admincommand.ErrRejected
		}
		remediation, resulting, outcome = "PENDING", "REMEDIATION_PENDING", "CARGO_REMEDIATION_REQUIRED"
	default:
		return admincommand.Transition{}, admincommand.ErrRejected
	}
	_, err = tx.Exec(ctx, `INSERT INTO hauling_cargo_suitability_decisions(booking_id,truck_id,decision,remediation_state,inputs_fingerprint,capability_version,fit_eligible,fit_reason,decision_id,version)
 VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9,1)
 ON CONFLICT (booking_id,truck_id) DO UPDATE SET decision=EXCLUDED.decision,remediation_state=EXCLUDED.remediation_state,inputs_fingerprint=EXCLUDED.inputs_fingerprint,
 capability_version=EXCLUDED.capability_version,fit_eligible=EXCLUDED.fit_eligible,fit_reason=EXCLUDED.fit_reason,evidence_ref='',decision_id=EXCLUDED.decision_id,
 version=hauling_cargo_suitability_decisions.version+1,updated_at=now()`, b.id, truckID, decision, remediation, fit.fingerprint, fit.capability, fit.eligible, fit.reason, in.DecisionID)
	if err != nil {
		return admincommand.Transition{}, err
	}
	if err := suitabilityHistory(ctx, tx, in, b.id, truckID, decision, remediation, fit, ""); err != nil {
		return admincommand.Transition{}, err
	}
	if err := advanceHead(ctx, tx, in, resulting, b.organization); err != nil {
		return admincommand.Transition{}, err
	}
	return admincommand.Transition{PriorVersion: in.ExpectedVersion, PriorState: orDefault(prior, "NO_DECISION"), ResultingState: resulting, Outcome: outcome}, nil
}

func (e *Executor) completeRemediation(ctx context.Context, tx pgx.Tx, in admincommand.Execution) (admincommand.Transition, error) {
	b, err := lockBooking(ctx, tx, in.SubjectID, true)
	if err != nil {
		return admincommand.Transition{}, err
	}
	if err := requireOptionalTenant(in, b.organization); err != nil {
		return admincommand.Transition{}, err
	}
	prior, err := fence(ctx, tx, in)
	if err != nil {
		return admincommand.Transition{}, err
	}
	if !suitabilityOpen(b.status) {
		return admincommand.Transition{}, admincommand.ErrRejected
	}
	var truckID string
	err = tx.QueryRow(ctx, `SELECT truck_id::text FROM hauling_cargo_suitability_decisions WHERE booking_id::text=$1 AND remediation_state='PENDING' FOR UPDATE`, b.id).Scan(&truckID)
	if errors.Is(err, pgx.ErrNoRows) {
		return admincommand.Transition{}, admincommand.ErrRejected
	}
	if err != nil {
		return admincommand.Transition{}, err
	}
	fit, err := e.currentFit(ctx, tx, b, truckID)
	if err != nil {
		return admincommand.Transition{}, err
	}
	if !fit.eligible {
		// Remediation cannot clear a truck that does not fit the cargo now.
		return admincommand.Transition{}, admincommand.ErrRejected
	}
	evidence := in.Params["evidence_ref"]
	if _, err := tx.Exec(ctx, `UPDATE hauling_cargo_suitability_decisions SET remediation_state='COMPLETED',evidence_ref=$3,inputs_fingerprint=$4,capability_version=$5,fit_eligible=true,fit_reason='',decision_id=$6,version=version+1,updated_at=now() WHERE booking_id::text=$1 AND truck_id::text=$2`, b.id, truckID, evidence, fit.fingerprint, fit.capability, in.DecisionID); err != nil {
		return admincommand.Transition{}, err
	}
	if err := suitabilityHistory(ctx, tx, in, b.id, truckID, decisionRemediation, "COMPLETED", fit, evidence); err != nil {
		return admincommand.Transition{}, err
	}
	if err := advanceHead(ctx, tx, in, "REMEDIATION_COMPLETED", b.organization); err != nil {
		return admincommand.Transition{}, err
	}
	return admincommand.Transition{PriorVersion: in.ExpectedVersion, PriorState: orDefault(prior, "REMEDIATION_PENDING"), ResultingState: "REMEDIATION_COMPLETED", Outcome: "CARGO_REMEDIATION_COMPLETED"}, nil
}

func suitabilityHistory(ctx context.Context, tx pgx.Tx, in admincommand.Execution, booking, truck, decision, remediation string, fit fitResult, evidence string) error {
	_, err := tx.Exec(ctx, `INSERT INTO hauling_cargo_suitability_history(booking_id,truck_id,action,decision,remediation_state,inputs_fingerprint,fit_eligible,fit_reason,evidence_ref,decision_id) VALUES($1::uuid,$2::uuid,$3,$4,$5,$6,$7,$8,$9,$10)`,
		booking, truck, in.Action, decision, remediation, fit.fingerprint, fit.eligible, fit.reason, evidence, in.DecisionID)
	return err
}

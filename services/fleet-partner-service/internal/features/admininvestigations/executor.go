// Package admininvestigations is Fleet's owner implementation of its closed
// administrative command dictionary (shared/go/admincommand specs.go):
// OPEN_INVESTIGATION, RECORD_FINDING and CLOSE_INVESTIGATION.
//
// The state machine is OPEN -> FINDING_RECORDED -> CLOSED. The case table is
// the record the rest of Fleet consults: the `investigations` admin resource
// and the Media case-authority proof read it, and the proof only serves the
// open states, so closing a case is what ends evidence access.
package admininvestigations

import (
	"context"
	"encoding/json"
	"errors"

	"cosmicforge/logistics/shared/go/admincommand"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const Owner = "fleet-partner-service"

// Case states and the pseudo-state reported before a case exists.
const (
	StateNone            = "NONE"
	StateOpen            = "OPEN"
	StateFindingRecorded = "FINDING_RECORDED"
	StateClosed          = "CLOSED"
	StateNoInvestigation = "NO_INVESTIGATION"
)

// subjectTables is the compiled map from enrolled subject kind to its
// authoritative Fleet table and the case column carrying the reference. No
// request selects a table.
var subjectTables = map[string]struct{ table, column, organizationColumn string }{
	"shipments":           {"fleet_partner_shipments", "shipment_id", "organization_id"},
	"private-assignments": {"fleet_partner_private_assignments", "private_assignment_id", "organization_id"},
	"execution-links":     {"fleet_partner_private_execution_operations", "execution_link_id", "organization_id"},
	"organizations":       {"fleet_partner_organizations", "organization_subject_id", "id"},
	"assets":              {"fleet_partner_assets", "asset_id", "organization_id"},
	"contracts":           {"fleet_partner_contracts", "contract_id", "organization_id"},
	"incidents":           {"fleet_partner_incidents", "incident_id", "organization_id"},
	"route-plans":         {"fleet_partner_route_plans", "route_plan_id", "organization_id"},
	"route-optimizations": {"fleet_partner_route_optimization_requests", "route_optimization_id", "organization_id"},
	"sla-measurements":    {"fleet_partner_sla_measurements", "sla_measurement_id", "organization_id"},
}

// Executor implements admincommand.Executor for Fleet.
type Executor struct{}

func Spec() admincommand.Spec {
	spec, _ := admincommand.ForOwner(Owner)
	return spec
}

// subjectOrganization locks the subject row and returns the owning
// organization from Fleet's authoritative record.
func subjectOrganization(ctx context.Context, tx pgx.Tx, kind, id string) (string, error) {
	target, ok := subjectTables[kind]
	if !ok {
		return "", admincommand.ErrNotFound
	}
	var org string
	err := tx.QueryRow(ctx, `SELECT `+target.organizationColumn+` FROM `+target.table+` WHERE id=$1 FOR SHARE`, id).Scan(&org)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", admincommand.ErrNotFound
	}
	return org, err
}

type caseRow struct {
	org, kind, subject, state string
	version                   int64
}

func (Executor) Apply(ctx context.Context, tx pgx.Tx, in admincommand.Execution) (admincommand.Transition, error) {
	var none admincommand.Transition
	if in.CaseRef == "" {
		return none, admincommand.ErrRejected
	}
	org, err := subjectOrganization(ctx, tx, in.SubjectKind, in.SubjectID)
	if err != nil {
		return none, err
	}
	// Fleet's canonical tenant is the organization. A subject owned by another
	// organization than the authorized scope is indistinguishable from absent.
	if org != in.Scope.Organization || org != in.Scope.Tenant {
		return none, admincommand.ErrNotFound
	}
	var current caseRow
	err = tx.QueryRow(ctx, `SELECT organization_id,subject_kind,subject_id,state,version FROM fleet_partner_admin_investigations WHERE case_ref=$1 FOR UPDATE`, in.CaseRef).Scan(&current.org, &current.kind, &current.subject, &current.state, &current.version)
	exists := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return none, err
	}
	if exists && (current.org != org || current.kind != in.SubjectKind || current.subject != in.SubjectID) {
		// A case reference of another subject or tenant is never disclosed.
		return none, admincommand.ErrNotFound
	}
	params, err := json.Marshal(in.Params)
	if err != nil {
		return none, err
	}
	history := func(prior, next string, version int64) error {
		var approval any
		if in.Approval != nil {
			approval = in.Approval.ApprovalID
		}
		_, err := tx.Exec(ctx, `INSERT INTO fleet_partner_admin_investigation_history(decision_id,case_ref,organization_id,action,prior_state,resulting_state,version,staff_ref,reason_code,params,approval_id,recorded_at)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, in.DecisionID, in.CaseRef, org, in.Action, prior, next, version, in.Staff, in.ReasonCode, params, approval, in.Now)
		return err
	}
	switch in.Action {
	case "OPEN_INVESTIGATION":
		if exists {
			if current.version != in.ExpectedVersion {
				return none, admincommand.ErrConflict
			}
			return none, admincommand.ErrRejected
		}
		if in.ExpectedVersion != 0 {
			return none, admincommand.ErrConflict
		}
		column := subjectTables[in.SubjectKind].column
		tag, err := tx.Exec(ctx, `INSERT INTO fleet_partner_admin_investigations(case_ref,organization_id,subject_kind,subject_id,`+column+`,category,severity,occurred_on,state,version,opened_by,created_at,updated_at)VALUES($1,$2,$3,$4,$4,$5,$6,$7::date,'OPEN',1,$8,$9,$9) ON CONFLICT (case_ref) DO NOTHING`, in.CaseRef, org, in.SubjectKind, in.SubjectID, in.Params["category"], in.Params["severity"], in.Params["occurred_on"], in.Staff, in.Now)
		if err != nil {
			return none, mapConstraint(err)
		}
		if tag.RowsAffected() != 1 {
			return none, admincommand.ErrConflict
		}
		if err := history(StateNone, StateOpen, 1); err != nil {
			return none, mapConstraint(err)
		}
		return admincommand.Transition{PriorVersion: 0, PriorState: StateNone, ResultingState: StateOpen, Outcome: "INVESTIGATION_OPENED"}, nil
	case "RECORD_FINDING", "CLOSE_INVESTIGATION":
		if !exists {
			return none, admincommand.ErrNotFound
		}
		if current.version != in.ExpectedVersion {
			return none, admincommand.ErrConflict
		}
		var next, outcome string
		var tag pgconn.CommandTag
		if in.Action == "RECORD_FINDING" {
			if current.state != StateOpen {
				return none, admincommand.ErrRejected
			}
			next, outcome = StateFindingRecorded, "FINDING_"+in.Params["finding"]
			var note any
			if v, ok := in.Params["note"]; ok {
				note = v
			}
			tag, err = tx.Exec(ctx, `UPDATE fleet_partner_admin_investigations SET state='FINDING_RECORDED',finding=$2,finding_note=$3,version=version+1,updated_at=$4 WHERE case_ref=$1 AND version=$5 AND state='OPEN'`, in.CaseRef, in.Params["finding"], note, in.Now, in.ExpectedVersion)
		} else {
			if current.state != StateFindingRecorded {
				return none, admincommand.ErrRejected
			}
			next, outcome = StateClosed, "INVESTIGATION_CLOSED_"+in.Params["closure"]
			tag, err = tx.Exec(ctx, `UPDATE fleet_partner_admin_investigations SET state='CLOSED',closure=$2,version=version+1,updated_at=$3 WHERE case_ref=$1 AND version=$4 AND state='FINDING_RECORDED'`, in.CaseRef, in.Params["closure"], in.Now, in.ExpectedVersion)
		}
		if err != nil {
			return none, mapConstraint(err)
		}
		if tag.RowsAffected() != 1 {
			return none, admincommand.ErrConflict
		}
		if err := history(current.state, next, in.ExpectedVersion+1); err != nil {
			return none, mapConstraint(err)
		}
		return admincommand.Transition{PriorVersion: current.version, PriorState: current.state, ResultingState: next, Outcome: outcome}, nil
	}
	return none, admincommand.ErrRejected
}

// Current reports the subject's most recently changed investigation. A
// subject without one is version 0 / NO_INVESTIGATION; an absent subject is
// not found.
func (Executor) Current(ctx context.Context, tx pgx.Tx, kind, id string) (int64, string, error) {
	target, ok := subjectTables[kind]
	if !ok {
		return 0, "", admincommand.ErrNotFound
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM `+target.table+` WHERE id=$1)`, id).Scan(&exists); err != nil {
		return 0, "", err
	}
	if !exists {
		return 0, "", admincommand.ErrNotFound
	}
	var version int64
	var state string
	err := tx.QueryRow(ctx, `SELECT version,state FROM fleet_partner_admin_investigations WHERE subject_kind=$1 AND subject_id=$2 ORDER BY updated_at DESC, case_ref DESC LIMIT 1`, kind, id).Scan(&version, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, StateNoInvestigation, nil
	}
	return version, state, err
}

func mapConstraint(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		switch pg.Code {
		case "23505":
			return admincommand.ErrConflict
		case "23514", "23503", "22007", "22008":
			return admincommand.ErrRejected
		}
	}
	return err
}

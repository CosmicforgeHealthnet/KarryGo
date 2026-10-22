package admincommands

import (
	"context"
	"errors"

	"cosmicforge/logistics/shared/go/admincommand"
	"github.com/jackc/pgx/v5"
)

// linkLegacyInspection maps a product-eligibility record's recorded opaque
// truck inspection_ref to a CONCLUDED modern inspection of the SAME truck.
// The supplied legacy_inspection_ref must byte-equal the stored reference: no
// alias, normalization, URL or Media object id is ever accepted in its place.
// One mapping per (truck, legacy ref); mappings are immutable.
func (e *Executor) linkLegacyInspection(ctx context.Context, tx pgx.Tx, in admincommand.Execution) (admincommand.Transition, error) {
	r, err := lockEligibility(ctx, tx, in.SubjectID, true)
	if err != nil {
		return admincommand.Transition{}, err
	}
	if err := requireTenant(in, r.organization); err != nil {
		return admincommand.Transition{}, err
	}
	prior, err := fence(ctx, tx, in)
	if err != nil {
		return admincommand.Transition{}, err
	}
	legacy := in.Params["legacy_inspection_ref"]
	if r.legacyRef == "" || legacy != r.legacyRef {
		return admincommand.Transition{}, admincommand.ErrRejected
	}
	var truckID, state, organization string
	err = tx.QueryRow(ctx, `SELECT truck_id::text,state,organization_id FROM hauling_admin_inspections WHERE id=$1 FOR SHARE`, in.CaseRef).Scan(&truckID, &state, &organization)
	if errors.Is(err, pgx.ErrNoRows) {
		return admincommand.Transition{}, admincommand.ErrRejected
	}
	if err != nil {
		return admincommand.Transition{}, err
	}
	if truckID != r.truckID || state != inspectionConcluded || organization != r.organization {
		return admincommand.Transition{}, admincommand.ErrRejected
	}
	tag, err := tx.Exec(ctx, `INSERT INTO hauling_admin_legacy_inspection_mappings(truck_id,legacy_inspection_ref,inspection_id,eligibility_id,decision_id) VALUES($1::uuid,$2,$3,$4::uuid,$5) ON CONFLICT (truck_id,legacy_inspection_ref) DO NOTHING`, r.truckID, legacy, in.CaseRef, r.eligibilityID, in.DecisionID)
	if err != nil {
		return admincommand.Transition{}, err
	}
	if tag.RowsAffected() != 1 {
		return admincommand.Transition{}, admincommand.ErrRejected
	}
	if err := advanceHead(ctx, tx, in, "LEGACY_LINKED", r.organization); err != nil {
		return admincommand.Transition{}, err
	}
	return admincommand.Transition{PriorVersion: in.ExpectedVersion, PriorState: orDefault(prior, "UNLINKED"), ResultingState: "LEGACY_LINKED", Outcome: "LEGACY_INSPECTION_LINKED"}, nil
}

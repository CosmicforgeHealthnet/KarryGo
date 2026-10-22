package admincommands

import (
	"context"
	"errors"

	"cosmicforge/logistics/services/hauling-service/internal/features/restrictions"
	"cosmicforge/logistics/shared/go/admincommand"
	"github.com/jackc/pgx/v5"
)

// adminSourcePrefix marks restriction rows Hauling applied on an owner
// administrative decision. A LIFT may clear only those: a restriction applied
// by another authority (Fleet, compliance) is that authority's to clear.
const adminSourcePrefix = "karryam-admin-"

// nextSourceVersion locks the organization-scoped restriction row (if any) and
// returns the source version the next transition must carry to take effect.
func nextSourceVersion(ctx context.Context, tx pgx.Tx, organization, typ, subject string) (int64, error) {
	var current int64
	err := tx.QueryRow(ctx, `SELECT source_version FROM hauling_authority_restrictions WHERE organization_id=$1 AND subject_type=$2 AND subject_reference=$3 FOR UPDATE`, organization, typ, subject).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return 1, nil
	}
	return current + 1, err
}

// applyRestriction applies (or re-asserts with Hauling provenance) the
// organization-scoped restriction the matcher and availability consult.
func applyRestriction(ctx context.Context, tx pgx.Tx, in admincommand.Execution, organization, typ, subject, reason, source string) (string, error) {
	version, err := nextSourceVersion(ctx, tx, organization, typ, subject)
	if err != nil {
		return "", err
	}
	op, err := restrictions.ApplyTx(ctx, tx, restrictions.Command{OperationReference: adminSourcePrefix + "op:" + in.DecisionID, OrganizationID: organization, SubjectType: typ, SubjectReference: subject, ReasonCode: reason, SourceReference: source, SourceVersion: version})
	if errors.Is(err, restrictions.ErrConflict) {
		return "", admincommand.ErrConflict
	}
	if err != nil {
		return "", err
	}
	if op.Outcome != "CONFIRMED" || op.RequestedState != restrictions.StateApplied {
		return "", admincommand.ErrConflict
	}
	return op.RestrictionID, nil
}

func outcomePrefix(typ string) string {
	if typ == subjectProvider {
		return "PROVIDER_"
	}
	return "TRUCK_"
}

func (e *Executor) restrict(ctx context.Context, tx pgx.Tx, in admincommand.Execution, typ string) (admincommand.Transition, error) {
	organization, _, err := ownerOrganization(ctx, tx, typ, in.SubjectID)
	if err != nil {
		return admincommand.Transition{}, err
	}
	if err := requireTenant(in, organization); err != nil {
		return admincommand.Transition{}, err
	}
	if _, err := fence(ctx, tx, in); err != nil {
		return admincommand.Transition{}, err
	}
	restricted, err := restrictedAny(ctx, tx, typ, in.SubjectID)
	if err != nil {
		return admincommand.Transition{}, err
	}
	if restricted {
		return admincommand.Transition{}, admincommand.ErrRejected
	}
	if _, err := applyRestriction(ctx, tx, in, organization, typ, in.SubjectID, in.Params["reason"], adminSourcePrefix+"command:"+in.DecisionID); err != nil {
		return admincommand.Transition{}, err
	}
	if err := advanceHead(ctx, tx, in, stateRestricted, organization); err != nil {
		return admincommand.Transition{}, err
	}
	return admincommand.Transition{PriorVersion: in.ExpectedVersion, PriorState: stateActive, ResultingState: stateRestricted, Outcome: outcomePrefix(typ) + "RESTRICTED"}, nil
}

func (e *Executor) lift(ctx context.Context, tx pgx.Tx, in admincommand.Execution, typ string) (admincommand.Transition, error) {
	organization, _, err := ownerOrganization(ctx, tx, typ, in.SubjectID)
	if err != nil {
		return admincommand.Transition{}, err
	}
	if err := requireTenant(in, organization); err != nil {
		return admincommand.Transition{}, err
	}
	if _, err := fence(ctx, tx, in); err != nil {
		return admincommand.Transition{}, err
	}
	var current restrictions.Command
	err = tx.QueryRow(ctx, `SELECT reason_code,source_version FROM hauling_authority_restrictions WHERE organization_id=$1 AND subject_type=$2 AND subject_reference=$3 AND state='APPLIED' AND source_reference LIKE 'karryam-admin-%' FOR UPDATE`, organization, typ, in.SubjectID).Scan(&current.ReasonCode, &current.SourceVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		// Nothing Hauling applied is in force for this owner: either the
		// subject is unrestricted or another authority owns the restriction.
		return admincommand.Transition{}, admincommand.ErrRejected
	}
	if err != nil {
		return admincommand.Transition{}, err
	}
	op, err := restrictions.ClearTx(ctx, tx, restrictions.Command{OperationReference: adminSourcePrefix + "op:" + in.DecisionID, OrganizationID: organization, SubjectType: typ, SubjectReference: in.SubjectID, ReasonCode: current.ReasonCode, SourceReference: adminSourcePrefix + "command:" + in.DecisionID, SourceVersion: current.SourceVersion + 1})
	if errors.Is(err, restrictions.ErrConflict) {
		return admincommand.Transition{}, admincommand.ErrConflict
	}
	if err != nil {
		return admincommand.Transition{}, err
	}
	if op.Outcome != "CONFIRMED" {
		return admincommand.Transition{}, admincommand.ErrConflict
	}
	still, err := restrictedAny(ctx, tx, typ, in.SubjectID)
	if err != nil {
		return admincommand.Transition{}, err
	}
	resulting := stateActive
	if still {
		resulting = stateRestricted
	}
	if err := advanceHead(ctx, tx, in, resulting, organization); err != nil {
		return admincommand.Transition{}, err
	}
	return admincommand.Transition{PriorVersion: in.ExpectedVersion, PriorState: stateRestricted, ResultingState: resulting, Outcome: outcomePrefix(typ) + "RESTRICTION_LIFTED"}, nil
}

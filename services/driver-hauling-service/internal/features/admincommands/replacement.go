package admincommands

import (
	"context"
	"errors"

	"cosmicforge/logistics/services/hauling-service/internal/features/recovery"
	"cosmicforge/logistics/shared/go/admincommand"
	"github.com/jackc/pgx/v5"
)

// requestReplacement binds an HQ-approved, operator-named replacement through
// recovery's own state machine, but only after CURRENT owner validation of the
// named provider and truck: active accounts, truck owned by the provider, no
// applied restriction, no blocking suitability decision for this booking, the
// same planning organization as the case, and a current cargofit pass.
func (e *Executor) requestReplacement(ctx context.Context, tx pgx.Tx, in admincommand.Execution) (admincommand.Transition, error) {
	c, err := lockRecovery(ctx, tx, in.SubjectID, true)
	if err != nil {
		return admincommand.Transition{}, err
	}
	if err := requireOptionalTenant(in, c.organization); err != nil {
		return admincommand.Transition{}, err
	}
	if _, err := fence(ctx, tx, in); err != nil {
		return admincommand.Transition{}, err
	}
	if c.state == string(recovery.StateResolved) || c.state == string(recovery.StateFailed) || c.replacementProvider != "" || e.Recovery == nil {
		return admincommand.Transition{}, admincommand.ErrRejected
	}
	provider, truck := in.Params["replacement_provider_id"], in.Params["replacement_truck_id"]
	var providerStatus, truckStatus, truckOwner string
	err = tx.QueryRow(ctx, `SELECT p.status,v.status,v.provider_id::text FROM truck_providers p, trucks v WHERE p.id::text=$1 AND v.id::text=$2`, provider, truck).Scan(&providerStatus, &truckStatus, &truckOwner)
	if errors.Is(err, pgx.ErrNoRows) {
		return admincommand.Transition{}, admincommand.ErrRejected
	}
	if err != nil {
		return admincommand.Transition{}, err
	}
	if providerStatus != "active" || truckStatus != "active" || truckOwner != provider {
		return admincommand.Transition{}, admincommand.ErrRejected
	}
	for typ, ref := range map[string]string{subjectProvider: provider, subjectAsset: truck} {
		restricted, err := restrictedAny(ctx, tx, typ, ref)
		if err != nil {
			return admincommand.Transition{}, err
		}
		if restricted {
			return admincommand.Transition{}, admincommand.ErrRejected
		}
	}
	isBlocked, err := blocked(ctx, tx, c.bookingID, provider, truck)
	if err != nil {
		return admincommand.Transition{}, err
	}
	if isBlocked {
		return admincommand.Transition{}, admincommand.ErrRejected
	}
	if c.organization != "" {
		providerOrg, _, err := ownerOrganization(ctx, tx, subjectProvider, provider)
		if err != nil {
			return admincommand.Transition{}, err
		}
		truckOrg, _, err := ownerOrganization(ctx, tx, subjectAsset, truck)
		if err != nil {
			return admincommand.Transition{}, err
		}
		if providerOrg != c.organization || truckOrg != c.organization {
			return admincommand.Transition{}, admincommand.ErrRejected
		}
	}
	advanced, err := e.Recovery.SelectNamedReplacementTx(ctx, tx, c.id, provider, truck, "karryam-admin:"+in.DecisionID)
	switch {
	case errors.Is(err, recovery.ErrNotFound):
		return admincommand.Transition{}, admincommand.ErrNotFound
	case errors.Is(err, recovery.ErrReplacementIneligible), errors.Is(err, recovery.ErrConflict), errors.Is(err, recovery.ErrUnavailable):
		return admincommand.Transition{}, admincommand.ErrRejected
	case err != nil:
		return admincommand.Transition{}, err
	}
	resulting := string(advanced.State)
	if err := advanceHead(ctx, tx, in, resulting, c.organization); err != nil {
		return admincommand.Transition{}, err
	}
	outcome := "REPLACEMENT_BOUND"
	if advanced.State == recovery.StateTransferRequired {
		outcome = "REPLACEMENT_AWAITING_CUSTODY_TRANSFER"
	}
	return admincommand.Transition{PriorVersion: in.ExpectedVersion, PriorState: c.state, ResultingState: resulting, Outcome: outcome}, nil
}

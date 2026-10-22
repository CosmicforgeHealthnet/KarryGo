// Package admincommands is Hauling's implementation of the closed owner
// administrative command dictionary (shared/go/admincommand, Admin Section 10):
// inspection cases and their verified Media associations, legacy inspection
// reference mapping, cargo suitability/remediation, truck/provider authority
// restrictions and operator-directed recovery replacement.
//
// Every action is a named transition of a Hauling-owned state machine executed
// inside the framework's transaction, and its effect lands in the tables the
// rest of the service already consults: restrictions in
// hauling_authority_restrictions (matching/availability), suitability in the
// gate the matcher and accept path read, replacements through recovery.
package admincommands

import (
	"context"
	"errors"

	"cosmicforge/logistics/services/hauling-service/internal/cargofit"
	"cosmicforge/logistics/services/hauling-service/internal/features/recovery"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/capabilityfit"
	"github.com/jackc/pgx/v5"
)

// FitEvaluator is the CURRENT truck fit authority (truckcapability.Service):
// the same evaluation recovery and preview use.
type FitEvaluator interface {
	EvaluateTruck(ctx context.Context, truckID string, requirement cargofit.Requirement) *capabilityfit.Decision
	ProductForRequirement(requirement cargofit.Requirement) (product string, certificationRequired bool)
}

// ReplacementBinder binds an operator-named replacement through recovery's
// own state machine inside the command transaction.
type ReplacementBinder interface {
	SelectNamedReplacementTx(ctx context.Context, tx pgx.Tx, caseID, providerID, truckID, actor string) (recovery.Case, error)
}

type Executor struct {
	Fit      FitEvaluator
	Recovery ReplacementBinder
}

func New(fit FitEvaluator, binder ReplacementBinder) *Executor {
	return &Executor{Fit: fit, Recovery: binder}
}

var _ admincommand.Executor = (*Executor)(nil)

func (e *Executor) Apply(ctx context.Context, tx pgx.Tx, in admincommand.Execution) (admincommand.Transition, error) {
	switch in.Action {
	case "RECORD_INSPECTION":
		return e.recordInspection(ctx, tx, in)
	case "ATTACH_INSPECTION_EVIDENCE":
		return e.attachEvidence(ctx, tx, in)
	case "CONCLUDE_INSPECTION":
		return e.concludeInspection(ctx, tx, in)
	case "RECORD_CARGO_SUITABILITY":
		return e.recordSuitability(ctx, tx, in)
	case "COMPLETE_CARGO_REMEDIATION":
		return e.completeRemediation(ctx, tx, in)
	case "RESTRICT_TRUCK":
		return e.restrict(ctx, tx, in, subjectAsset)
	case "LIFT_TRUCK_RESTRICTION":
		return e.lift(ctx, tx, in, subjectAsset)
	case "RESTRICT_PROVIDER":
		return e.restrict(ctx, tx, in, subjectProvider)
	case "LIFT_PROVIDER_RESTRICTION":
		return e.lift(ctx, tx, in, subjectProvider)
	case "REQUEST_RECOVERY_REPLACEMENT":
		return e.requestReplacement(ctx, tx, in)
	case "LINK_LEGACY_INSPECTION_REFERENCE":
		return e.linkLegacyInspection(ctx, tx, in)
	}
	return admincommand.Transition{}, admincommand.ErrRejected
}

// Current reports the subject's administrative version and its REAL current
// owner state (restriction rows, native recovery state), not a cached label.
func (e *Executor) Current(ctx context.Context, tx pgx.Tx, kind, id string) (int64, string, error) {
	version, headState, _, err := readHead(ctx, tx, kind, id, false)
	if err != nil {
		return 0, "", err
	}
	switch kind {
	case "trucks", "providers":
		typ := subjectAsset
		if kind == "providers" {
			typ = subjectProvider
		}
		if _, _, err := ownerOrganization(ctx, tx, typ, id); err != nil {
			return 0, "", err
		}
		restricted, err := restrictedAny(ctx, tx, typ, id)
		if err != nil {
			return 0, "", err
		}
		if restricted {
			return version, stateRestricted, nil
		}
		return version, stateActive, nil
	case "booking-cargo-inputs":
		if _, err := lockBooking(ctx, tx, id, false); err != nil {
			return 0, "", err
		}
		return version, orDefault(headState, "NO_DECISION"), nil
	case "recoveries":
		c, err := lockRecovery(ctx, tx, id, false)
		if err != nil {
			return 0, "", err
		}
		return version, c.state, nil
	case "product-eligibility":
		if _, err := lockEligibility(ctx, tx, id, false); err != nil {
			return 0, "", err
		}
		return version, orDefault(headState, "UNLINKED"), nil
	}
	return 0, "", admincommand.ErrNotFound
}

func orDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// mapNotFound converts a missing row into the framework's not-found outcome.
func mapNotFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return admincommand.ErrNotFound
	}
	return err
}

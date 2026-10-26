// Package admincommands executes Verification's closed Secure Admin review
// dictionary (admincommand.VerificationSpec) against the native vendor case
// state machine. The Bridge only carries a validated, approved intent; this
// owner locks the case, fences on source_version, applies the transition,
// names the human reviewer in the append-only history and queues the existing
// signed Vendor callback. Nothing here writes Vendor state.
package admincommands

import (
	"context"
	"errors"
	"strings"
	"time"

	"cosmicforge/logistics/services/verification-compliance-service/internal/features/adminresources"
	"cosmicforge/logistics/services/verification-compliance-service/internal/features/vendors"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/adminowner"
	"github.com/jackc/pgx/v5"
)

const SubjectKind = "vendor-cases"

type Executor struct{}

func Spec() admincommand.Spec { return admincommand.VerificationSpec() }

func (Executor) AuthorizeStaff(ctx context.Context, tx pgx.Tx, staff, kind, id string) error {
	return adminresources.Source().AuthorizeStaff(ctx, tx, staff, kind, id)
}

func (Executor) Current(ctx context.Context, tx pgx.Tx, kind, id string) (int64, string, error) {
	item, err := adminresources.Source().Read(ctx, tx, kind, id, false)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "", admincommand.ErrNotFound
	}
	if err != nil {
		return 0, "", err
	}
	if item.SourceVersion == nil || item.Status == nil {
		return 0, "", admincommand.ErrNotFound
	}
	return *item.SourceVersion, strings.ToUpper(*item.Status), nil
}

// permissionFor separates review handling from final decisions.
func permissionFor(action string) string {
	switch action {
	case "START_REVIEW", "REQUEST_REMEDIATION":
		return adminresources.PermissionReview
	}
	return adminresources.PermissionDecide
}

// Decision maps one dictionary action onto a native decision input.
func Decision(in admincommand.Execution) (vendors.DecisionInput, error) {
	d := vendors.DecisionInput{CaseRef: in.SubjectID, ExpectedVersion: in.ExpectedVersion, DecidedBy: in.Staff, DecidedByKind: vendors.DecidedByStaff, OperationRef: in.OperationID, Now: in.Now.UTC()}
	switch in.Action {
	case "START_REVIEW":
		d.Status = vendors.StatusInProgress
	case "REQUEST_REMEDIATION":
		d.Status, d.ReasonCode, d.Remediation = vendors.StatusPendingReview, in.ReasonCode, in.Params["remediation"]
	case "APPROVE_CASE":
		on, err := time.Parse("2006-01-02", in.Params["expires_on"])
		if err != nil {
			return d, admincommand.ErrRejected
		}
		expires := on.UTC()
		d.Status, d.ExpiresAt = vendors.StatusVerified, &expires
	case "REJECT_CASE":
		d.Status, d.ReasonCode = vendors.StatusRejected, in.Params["reason"]
	case "SUSPEND_CASE":
		d.Status, d.ReasonCode = vendors.StatusSuspended, in.Params["reason"]
	case "REOPEN_CASE":
		d.Status, d.AppealRef = vendors.StatusPendingReview, in.Params["appeal_ref"]
	default:
		return d, admincommand.ErrRejected
	}
	return d, nil
}

func (Executor) Apply(ctx context.Context, tx pgx.Tx, in admincommand.Execution) (admincommand.Transition, error) {
	var out admincommand.Transition
	if in.SubjectKind != SubjectKind {
		return out, admincommand.ErrNotFound
	}
	if err := adminresources.RequireMutationGrant(ctx, tx, in.Staff, permissionFor(in.Action)); err != nil {
		return out, admincommand.ErrNotFound
	}
	decision, err := Decision(in)
	if err != nil {
		return out, err
	}
	result, err := vendors.ApplyDecision(ctx, tx, decision)
	switch {
	case errors.Is(err, vendors.ErrCaseNotFound):
		return out, admincommand.ErrNotFound
	case errors.Is(err, vendors.ErrVersionConflict):
		return out, admincommand.ErrConflict
	case errors.Is(err, vendors.ErrIllegalTransition), errors.Is(err, vendors.ErrReasonRequired), errors.Is(err, vendors.ErrInvalidReason),
		errors.Is(err, vendors.ErrFutureExpiryRequired), errors.Is(err, vendors.ErrAppealRequired), errors.Is(err, vendors.ErrAppealNotApplicable), errors.Is(err, vendors.ErrAppealReused):
		return out, admincommand.ErrRejected
	case err != nil:
		return out, err
	}
	return admincommand.Transition{PriorVersion: result.Prior.SourceVersion, PriorState: result.Prior.Status, ResultingState: result.Current.Status, Outcome: "APPLIED_BY_VERIFICATION_OWNER"}, nil
}

var _ adminowner.StaffAuthorizer = Executor{}
var _ admincommand.Executor = Executor{}

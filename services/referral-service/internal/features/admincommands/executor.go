package admincommands

import (
	"context"
	"errors"
	"strings"

	"cosmicforge/logistics/services/referral-service/internal/features/adminresources"
	referralusecases "cosmicforge/logistics/services/referral-service/internal/features/referrals/usecases"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/apperrors"
	"github.com/jackc/pgx/v5"
)

type Executor struct{ Referral *referralusecases.Service }

func Spec() admincommand.Spec { return admincommand.ReferralSpec() }

func (e Executor) AuthorizeStaff(ctx context.Context, tx pgx.Tx, staff, kind, id string) error {
	return adminresources.Source().AuthorizeStaff(ctx, tx, staff, kind, id)
}

func (e Executor) Current(ctx context.Context, tx pgx.Tx, kind, id string) (int64, string, error) {
	item, err := adminresources.Source().Read(ctx, tx, kind, id, false)
	if err != nil {
		return 0, "", err
	}
	if item.SourceVersion == nil || item.Status == nil {
		return 0, "", admincommand.ErrNotFound
	}
	return *item.SourceVersion, strings.ToUpper(*item.Status), nil
}

func (e Executor) Apply(ctx context.Context, tx pgx.Tx, in admincommand.Execution) (admincommand.Transition, error) {
	var out admincommand.Transition
	if e.Referral == nil {
		return out, admincommand.ErrDisabled
	}
	permission := "referral.review.manage"
	if in.Action == "REVERSE_REWARD" {
		permission = "referral.reward.reverse"
	}
	if err := adminresources.RequireMutationGrant(ctx, tx, in.Staff, permission); err != nil {
		return out, admincommand.ErrNotFound
	}
	version, prior, next, err := e.Referral.ApplyAdminCommandInTx(ctx, tx, in.Action, in.SubjectID, in.ExpectedVersion, referralusecases.AdminRequest{Caller: "admin-backoffice-service", PerformedBy: in.Staff, Reason: in.ReasonCode}, in.Params)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, admincommand.ErrNotFound
		}
		var app *apperrors.Error
		if errors.As(err, &app) {
			switch app.Status {
			case 404:
				return out, admincommand.ErrNotFound
			case 409:
				return out, admincommand.ErrConflict
			default:
				if app.Status >= 400 && app.Status < 500 {
					return out, admincommand.ErrRejected
				}
			}
		}
		return out, err
	}
	return admincommand.Transition{PriorVersion: version, PriorState: strings.ToUpper(prior), ResultingState: strings.ToUpper(next), Outcome: "APPLIED_BY_REFERRAL_OWNER"}, nil
}

var _ adminowner.StaffAuthorizer = Executor{}

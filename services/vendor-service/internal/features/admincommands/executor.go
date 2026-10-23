package admincommands

import (
	"context"
	"cosmicforge/logistics/services/vendor-service/internal/features/adminresources"
	"cosmicforge/logistics/services/vendor-service/internal/features/food"
	"cosmicforge/logistics/services/vendor-service/internal/features/orders"
	"cosmicforge/logistics/services/vendor-service/internal/features/production"
	"cosmicforge/logistics/services/vendor-service/internal/features/shop"
	"cosmicforge/logistics/services/vendor-service/internal/features/vendors"
	"cosmicforge/logistics/shared/go/admincommand"
	"cosmicforge/logistics/shared/go/adminowner"
	"cosmicforge/logistics/shared/go/apperrors"
	"errors"
	"github.com/jackc/pgx/v5"
	"strconv"
	"time"
)

const Owner = "vendor-service"

type Executor struct {
	Vendor     *vendors.Service
	Food       *food.Service
	Shop       *shop.Service
	Production *production.Service
	Orders     *orders.Service
}

func Spec() admincommand.Spec { return admincommand.VendorSpec() }
func (e Executor) AuthorizeStaff(ctx context.Context, tx pgx.Tx, staff, kind, id string) error {
	return adminresources.Source().AuthorizeStaff(ctx, tx, staff, kind, id)
}
func (e Executor) Current(ctx context.Context, tx pgx.Tx, kind, id string) (int64, string, error) {
	item, err := adminresources.Source().Read(ctx, tx, kind, id, false)
	if err != nil {
		return 0, "", err
	}
	version := int64(0)
	state := "UNKNOWN"
	if item.SourceVersion != nil {
		version = *item.SourceVersion
	}
	if item.Status != nil {
		state = *item.Status
	}
	return version, state, nil
}
func (e Executor) Apply(ctx context.Context, tx pgx.Tx, in admincommand.Execution) (admincommand.Transition, error) {
	var out admincommand.Transition
	business, branch, _, err := adminresources.NativeTarget(ctx, tx, in.SubjectKind, in.SubjectID)
	if err != nil {
		return out, err
	}
	if in.Spec.VersionOf == admincommand.VersionCase {
		permission := map[string]string{"merchants": "vendor.business.manage", "branches": "vendor.branches.manage", "food-items": "vendor.food.catalogue.manage", "shop-listings": "vendor.shop.listings.manage", "stock-balances": "vendor.inventory.manage", "production-jobs": "vendor.production.operate", "production-exceptions": "vendor.production.operate", "capacity-reservations": "vendor.production.operate", "vendor-orders": "vendor.orders.manage"}[in.SubjectKind]
		if permission == "" {
			return out, admincommand.ErrRejected
		}
		if err = adminresources.RequireNativeStaffForMutation(ctx, tx, in.Staff, business, branch, permission); err != nil {
			return out, admincommand.ErrNotFound
		}
		return applyCase(ctx, tx, in, business, branch)
	}
	currentVersion, _, err := e.Current(ctx, tx, in.SubjectKind, in.SubjectID)
	if err != nil {
		return out, err
	}
	if currentVersion != in.ExpectedVersion {
		return out, admincommand.ErrConflict
	}
	permission := ""
	switch in.Action {
	case "PROCESS_APPROVED_SUBSTITUTION":
		permission = "vendor.orders.manage"
	case "PAUSE_MERCHANT", "REOPEN_MERCHANT":
		permission = "vendor.business.manage"
	case "PAUSE_BRANCH", "REOPEN_BRANCH":
		permission = "vendor.branches.manage"
	case "BLOCK_PRODUCTION", "RESUME_PRODUCTION", "REPORT_PRODUCTION_DELAY", "RESCHEDULE_CAPACITY":
		permission = "vendor.production.operate"
	case "TAKE_DOWN_LISTING", "RESTORE_LISTING":
		if in.SubjectKind == "food-items" {
			permission = "vendor.food.catalogue.manage"
		} else {
			permission = "vendor.shop.listings.manage"
		}
	default:
		return out, admincommand.ErrRejected
	}
	if err = adminresources.RequireNativeStaffForMutation(ctx, tx, in.Staff, business, branch, permission); err != nil {
		return out, admincommand.ErrNotFound
	}
	var version int64
	var prior, next string
	switch in.Action {
	case "PROCESS_APPROVED_SUBSTITUTION":
		if e.Orders == nil {
			return out, admincommand.ErrDisabled
		}
		if err = adminresources.RequireNativeStaffForMutation(ctx, tx, in.Staff, business, branch, "vendor.inventory.manage"); err != nil {
			return out, admincommand.ErrNotFound
		}
		version, next, err = e.Orders.ProcessApprovedSubstitutionInTx(ctx, tx, orders.Actor{StaffID: in.Staff, VendorID: business}, in.SubjectID, in.ExpectedVersion)
		prior = "APPROVED"

	case "PAUSE_MERCHANT", "REOPEN_MERCHANT", "PAUSE_BRANCH", "REOPEN_BRANCH":
		if e.Vendor == nil {
			return out, admincommand.ErrDisabled
		}
		version, prior, next, err = e.Vendor.AdministrativeOperationalStatusTx(ctx, tx, vendors.Actor{StaffID: in.Staff, VendorID: business, CorrelationID: in.OperationID}, branch, in.Action == "REOPEN_MERCHANT" || in.Action == "REOPEN_BRANCH", in.ExpectedVersion)
	case "BLOCK_PRODUCTION", "RESUME_PRODUCTION", "REPORT_PRODUCTION_DELAY", "RESCHEDULE_CAPACITY":
		if e.Production == nil {
			return out, admincommand.ErrDisabled
		}
		actor := production.Actor{StaffID: in.Staff, VendorID: business, CommandRef: in.IdempotencyKey, CorrelationID: in.OperationID}
		scope := production.Scope{BranchIDs: []string{branch}}
		if in.Action == "RESCHEDULE_CAPACITY" {
			start, a := strconv.ParseInt(in.Params["window_start_unix"], 10, 64)
			end, b := strconv.ParseInt(in.Params["window_end_unix"], 10, 64)
			if a != nil || b != nil || start <= 0 || end <= start {
				return out, admincommand.ErrRejected
			}
			_, prior, err = e.Current(ctx, tx, in.SubjectKind, in.SubjectID)
			if err != nil {
				return out, err
			}
			_, _, err = e.Production.RescheduleInTx(ctx, tx, actor, scope, in.SubjectID, production.RescheduleInput{ExpectedVersion: in.ExpectedVersion, WindowStart: time.Unix(start, 0).UTC(), WindowEnd: time.Unix(end, 0).UTC()})
			version, next = in.ExpectedVersion, prior
		} else if in.Action == "REPORT_PRODUCTION_DELAY" {
			_, prior, err = e.Current(ctx, tx, in.SubjectKind, in.SubjectID)
			if err != nil {
				return out, err
			}
			if prior != "PLANNED" && prior != "QUEUED" && prior != "IN_PROGRESS" && prior != "BLOCKED" {
				return out, admincommand.ErrRejected
			}
			start, parseStart := strconv.ParseInt(in.Params["window_start_unix"], 10, 64)
			end, parseEnd := strconv.ParseInt(in.Params["window_end_unix"], 10, 64)
			if parseStart != nil || parseEnd != nil || start <= 0 || end <= start {
				return out, admincommand.ErrRejected
			}
			from, to := time.Unix(start, 0).UTC(), time.Unix(end, 0).UTC()
			err = e.Production.ReportDelayInTx(ctx, tx, actor, in.SubjectID, production.DelayInput{ExpectedVersion: in.ExpectedVersion, ReasonCode: in.Params["reason"], NewWindowStart: &from, NewWindowEnd: &to, IdempotencyKey: in.IdempotencyKey})
			version, next = in.ExpectedVersion, prior
		} else {
			version, prior, next, err = e.Production.AdministrativeInterruptionTx(ctx, tx, actor, scope, in.SubjectID, in.Action == "RESUME_PRODUCTION", in.ReasonCode, in.ExpectedVersion)
		}
	case "TAKE_DOWN_LISTING", "RESTORE_LISTING":
		if in.SubjectKind == "food-items" {
			if e.Food == nil {
				return out, admincommand.ErrDisabled
			}
			version, prior, next, err = e.Food.AdministrativeListingTx(ctx, tx, food.Actor{StaffID: in.Staff, VendorID: business, CommandRef: in.IdempotencyKey, CorrelationID: in.OperationID}, food.Scope{BranchIDs: []string{branch}}, in.SubjectID, in.Action == "RESTORE_LISTING", in.ExpectedVersion)
		} else {
			if e.Shop == nil {
				return out, admincommand.ErrDisabled
			}
			version, prior, next, err = e.Shop.AdministrativeListingTx(ctx, tx, shop.Actor{StaffID: in.Staff, VendorID: business, CommandRef: in.IdempotencyKey, CorrelationID: in.OperationID}, shop.Scope{BranchIDs: []string{branch}}, in.SubjectID, in.Action == "RESTORE_LISTING", in.ExpectedVersion)
		}
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return out, admincommand.ErrNotFound
	}
	if errors.Is(err, orders.ErrVersionConflict) {
		return out, admincommand.ErrConflict
	}
	if errors.Is(err, orders.ErrOrderConflict) || errors.Is(err, orders.ErrForbidden) {
		return out, admincommand.ErrRejected
	}
	if errors.Is(err, orders.ErrOrderNotFound) {
		return out, admincommand.ErrNotFound
	}
	if errors.Is(err, orders.ErrDependencyMissing) || errors.Is(err, orders.ErrDisabled) {
		return out, admincommand.ErrDisabled
	}
	if err != nil {
		var app *apperrors.Error
		if errors.As(err, &app) && app.Status >= 400 && app.Status < 500 {
			if app.Status == 409 {
				return out, admincommand.ErrConflict
			}
			return out, admincommand.ErrRejected
		}
		return out, err
	}
	actualVersion, actualState, readErr := e.Current(ctx, tx, in.SubjectKind, in.SubjectID)
	if readErr != nil {
		return out, readErr
	}
	if actualVersion != version+1 || actualState != next {
		return out, admincommand.ErrConflict
	}
	outcome := "APPLIED_BY_OWNER"
	if in.Action == "PROCESS_APPROVED_SUBSTITUTION" {
		outcome = "APPROVED_SUBSTITUTION_INVENTORY_RECONCILED"
	}
	return admincommand.Transition{PriorVersion: version, PriorState: prior, ResultingState: next, Outcome: outcome}, nil
}

var _ adminowner.StaffAuthorizer = Executor{}

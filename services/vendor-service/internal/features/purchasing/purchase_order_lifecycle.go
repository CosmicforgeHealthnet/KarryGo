package purchasing

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

func (s *PurchaseOrderService) approvalScopeForOrder(
	ctx context.Context,
	actor Actor,
	order PurchaseOrder,
) (
	Scope,
	error,
) {
	if order.ApprovalPermissionCode == "" ||
		!allowedApprovalPermission(
			order.ApprovalPermissionCode,
		) {

		return Scope{},
			apperrors.Conflict(
				"Purchase order has no valid approval authority snapshot.",
				nil,
			)
	}

	scope, err :=
		s.writeScope(
			ctx,
			actor,
			order.ApprovalPermissionCode,
		)

	if err != nil {
		return Scope{}, err
	}

	switch order.ApprovalScope {
	case ApprovalScopeBusinessOnly:
		if !scope.Global {
			return Scope{},
				apperrors.Forbidden(
					"This purchase order requires business-wide approval authority.",
					nil,
				)
		}

	case ApprovalScopeBranchOrBusiness:
		if err =
			requireBranch(
				scope,
				order.BranchID,
			); err != nil {

			return Scope{}, err
		}

	default:
		return Scope{},
			apperrors.Conflict(
				"Purchase order has an invalid approval-scope snapshot.",
				nil,
			)
	}

	return scope, nil
}

func (s *PurchaseOrderService) SubmitPurchaseOrder(
	ctx context.Context,
	actor Actor,
	purchaseOrderID string,
	input PurchaseOrderTransitionInput,
) (
	PurchaseOrder,
	error,
) {
	scope, err :=
		s.writeScope(
			ctx,
			actor,
			PermissionManage,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	requestHash, err :=
		purchaseOrderCommandHash(
			struct {
				PurchaseOrderID string `json:"purchase_order_id"`
				ExpectedVersion int64  `json:"expected_version"`
			}{
				PurchaseOrderID: strings.TrimSpace(
					purchaseOrderID,
				),

				ExpectedVersion: input.ExpectedVersion,
			},
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	tx, err :=
		begin(
			ctx,
			s.base.db,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if replay,
		ok,
		replayErr :=
		s.replayPurchaseOrderCommand(
			ctx,
			tx,
			actor,
			purchaseOrderID,
			"SUBMIT",
			requestHash,
		); replayErr != nil {

		return PurchaseOrder{},
			replayErr

	} else if ok {
		if err =
			tx.Commit(
				ctx,
			); err != nil {

			return PurchaseOrder{}, err
		}

		return replay, nil
	}

	current, err :=
		s.loadPurchaseOrder(
			ctx,
			tx,
			actor.VendorID,
			purchaseOrderID,
			true,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	if err =
		requireBranch(
			scope,
			current.BranchID,
		); err != nil {

		return PurchaseOrder{}, err
	}

	if input.ExpectedVersion <= 0 ||
		current.Version !=
			input.ExpectedVersion {

		return PurchaseOrder{},
			versionConflict()
	}

	if current.Status !=
		PurchaseOrderDraft {

		return PurchaseOrder{},
			apperrors.Conflict(
				"Only a DRAFT purchase order can be submitted.",
				nil,
			)
	}

	var lineCount int

	if err =
		tx.QueryRow(
			ctx,
			`SELECT count(*)
               FROM vendor_purchase_order_lines
              WHERE business_id=$1
                AND purchase_order_id=$2`,
			current.BusinessID,
			current.ID,
		).Scan(
			&lineCount,
		); err != nil {

		return PurchaseOrder{}, err
	}

	if lineCount == 0 {
		return PurchaseOrder{},
			apperrors.Conflict(
				"Purchase order has no lines.",
				nil,
			)
	}

	policy,
		rule,
		err :=
		s.effectiveApprovalRuleTx(
			ctx,
			tx,
			current.BusinessID,
			current.BranchID,
			current.TotalAmountMinor,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	now :=
		time.Now().
			UTC()

	order, err :=
		scanPurchaseOrder(
			tx.QueryRow(
				ctx,
				`UPDATE vendor_purchase_orders
                    SET status='SUBMITTED',
                        submitted_by_staff_id=$3,
                        submitted_at=$4,

                        approval_policy_id=$5,
                        approval_policy_version=$6,
                        approval_rule_id=$7,
                        approval_rule_version=$8,

                        required_approval_count=$9,
                        approval_permission_code=$10,
                        approval_scope=$11,
                        self_approval_allowed=$12,
                        approval_step_up_required=$13,

                        approved_at=NULL,
                        rejected_at=NULL,

                        version=version+1,
                        updated_at=$4

                  WHERE business_id=$1
                    AND id=$2
                  RETURNING `+
					purchaseOrderColumns,
				current.BusinessID,
				current.ID,
				actor.StaffID,
				now,
				policy.ID,
				policy.Version,
				rule.ID,
				rule.Version,
				rule.RequiredApprovalCount,
				rule.RequiredPermissionCode,
				rule.ApprovalScope,
				policy.AllowSelfApproval,
				rule.StepUpRequired,
			),
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	order, err =
		s.hydratePurchaseOrder(
			ctx,
			tx,
			order,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	if err =
		s.appendStatusHistory(
			ctx,
			tx,
			actor,
			order,
			current.Status,
			order.Status,
			input.Note,
		); err != nil {

		return PurchaseOrder{}, err
	}

	if err =
		s.base.recordMutation(
			ctx,
			tx,
			actor,
			"purchasing.purchase_order.submitted",
			"vendor.purchasing.purchase_order.submitted",
			"vendor_purchase_order",
			order.ID,
			order.BranchID,
			order.Version,
			current,
			order,
			map[string]any{
				"purchase_order_id":        order.ID,
				"status":                   order.Status,
				"total_amount_minor":       order.TotalAmountMinor,
				"required_approval_count":  order.RequiredApprovalCount,
				"approval_permission_code": order.ApprovalPermissionCode,
				"approval_scope":           order.ApprovalScope,
				"approval_generation":      order.ApprovalGeneration,
			},
		); err != nil {

		return PurchaseOrder{}, err
	}

	if err =
		s.insertPurchaseOrderCommand(
			ctx,
			tx,
			actor,
			order,
			"SUBMIT",
			requestHash,
		); err != nil {

		return PurchaseOrder{}, err
	}

	if err =
		tx.Commit(
			ctx,
		); err != nil {

		return PurchaseOrder{}, err
	}

	return order, nil
}

func (s *PurchaseOrderService) approveOrRejectPurchaseOrder(
	ctx context.Context,
	actor Actor,
	purchaseOrderID string,
	input PurchaseOrderApprovalInput,
	decision string,
) (
	PurchaseOrder,
	error,
) {
	commandType :=
		"APPROVE"

	if decision ==
		ApprovalDecisionRejected {

		commandType =
			"REJECT"
	}

	requestHash, err :=
		purchaseOrderCommandHash(
			struct {
				PurchaseOrderID string `json:"purchase_order_id"`
				ExpectedVersion int64  `json:"expected_version"`
				Decision        string `json:"decision"`
				StepUpGrantID   string `json:"step_up_grant_id"`
				Note            string `json:"note"`
			}{
				PurchaseOrderID: strings.TrimSpace(
					purchaseOrderID,
				),

				ExpectedVersion: input.ExpectedVersion,

				Decision: decision,

				StepUpGrantID: strings.TrimSpace(
					input.StepUpGrantID,
				),

				Note: strings.TrimSpace(
					input.Note,
				),
			},
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	tx, err :=
		begin(
			ctx,
			s.base.db,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if replay,
		ok,
		replayErr :=
		s.replayPurchaseOrderCommand(
			ctx,
			tx,
			actor,
			purchaseOrderID,
			commandType,
			requestHash,
		); replayErr != nil {

		return PurchaseOrder{},
			replayErr

	} else if ok {
		if err =
			tx.Commit(
				ctx,
			); err != nil {

			return PurchaseOrder{}, err
		}

		return replay, nil
	}

	current, err :=
		s.loadPurchaseOrder(
			ctx,
			tx,
			actor.VendorID,
			purchaseOrderID,
			true,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	if input.ExpectedVersion <= 0 ||
		current.Version !=
			input.ExpectedVersion {

		return PurchaseOrder{},
			versionConflict()
	}

	if current.Status !=
		PurchaseOrderSubmitted {

		return PurchaseOrder{},
			apperrors.Conflict(
				"Only a SUBMITTED purchase order can be approved or rejected.",
				nil,
			)
	}

	if _, err =
		s.approvalScopeForOrder(
			ctx,
			actor,
			current,
		); err != nil {

		return PurchaseOrder{}, err
	}

	if !current.SelfApprovalAllowed &&
		(actor.StaffID ==
			current.CreatedByStaffID ||
			actor.StaffID ==
				current.SubmittedByStaffID) {

		return PurchaseOrder{},
			apperrors.Forbidden(
				"This Vendor's approval policy does not allow self-approval.",
				nil,
			)
	}

	stepUpConsumed :=
		false

	if current.ApprovalStepUpRequired {
		grantID :=
			strings.TrimSpace(
				input.StepUpGrantID,
			)

		if grantID == "" ||
			s.stepUp == nil {

			return PurchaseOrder{},
				apperrors.Forbidden(
					"Step-up authentication is required for this approval.",
					nil,
				)
		}

		if err =
			s.stepUp.ConsumeGrant(
				ctx,
				grantID,
				actor.SessionID,
				actor.StaffID,
				actor.VendorID,
				StepUpPurposePurchaseOrderApprove,
			); err != nil {

			return PurchaseOrder{}, err
		}

		stepUpConsumed = true
	}

	_, err =
		tx.Exec(
			ctx,
			`INSERT INTO vendor_purchase_order_approvals(
                id,
                business_id,
                purchase_order_id,
                approval_generation,
                approver_staff_id,
                decision,
                permission_code_snapshot,
                approval_scope_snapshot,
                amount_minor_snapshot,
                purchase_order_version_before,
                step_up_consumed,
                note
            )
            VALUES(
                $1,$2,$3,$4,$5,$6,
                $7,$8,$9,$10,$11,$12
            )`,
			"vpoa_"+
				uuid.NewString(),
			current.BusinessID,
			current.ID,
			current.ApprovalGeneration,
			actor.StaffID,
			decision,
			current.ApprovalPermissionCode,
			current.ApprovalScope,
			current.TotalAmountMinor,
			current.Version,
			stepUpConsumed,
			strings.TrimSpace(
				input.Note,
			),
		)

	if err != nil {
		return PurchaseOrder{},
			mapWriteError(err)
	}

	nextStatus :=
		current.Status

	var approvedAt any

	var rejectedAt any

	if decision ==
		ApprovalDecisionRejected {

		nextStatus =
			PurchaseOrderRejected

		rejectedAt =
			time.Now().
				UTC()

	} else {
		var approvalCount int

		if err =
			tx.QueryRow(
				ctx,
				`SELECT count(*)
                   FROM vendor_purchase_order_approvals
                  WHERE business_id=$1
                    AND purchase_order_id=$2
                    AND approval_generation=$3
                    AND decision='APPROVED'`,
				current.BusinessID,
				current.ID,
				current.ApprovalGeneration,
			).Scan(
				&approvalCount,
			); err != nil {

			return PurchaseOrder{}, err
		}

		if approvalCount >=
			current.RequiredApprovalCount {

			nextStatus =
				PurchaseOrderApproved

			approvedAt =
				time.Now().
					UTC()
		}
	}

	order, err :=
		scanPurchaseOrder(
			tx.QueryRow(
				ctx,
				`UPDATE vendor_purchase_orders
                    SET status=$3,
                        approved_at=
                            CASE
                                WHEN $3='APPROVED'
                                THEN COALESCE($4::timestamptz,approved_at)
                                ELSE approved_at
                            END,
                        rejected_at=
                            CASE
                                WHEN $3='REJECTED'
                                THEN $5::timestamptz
                                ELSE rejected_at
                            END,
                        version=version+1,
                        updated_at=now()
                  WHERE business_id=$1
                    AND id=$2
                  RETURNING `+
					purchaseOrderColumns,
				current.BusinessID,
				current.ID,
				nextStatus,
				approvedAt,
				rejectedAt,
			),
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	order, err =
		s.hydratePurchaseOrder(
			ctx,
			tx,
			order,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	if nextStatus != current.Status {
		if err =
			s.appendStatusHistory(
				ctx,
				tx,
				actor,
				order,
				current.Status,
				nextStatus,
				input.Note,
			); err != nil {

			return PurchaseOrder{}, err
		}
	}

	action :=
		"purchasing.purchase_order.approval_recorded"

	eventType :=
		"vendor.purchasing.purchase_order.approval_recorded"

	if decision ==
		ApprovalDecisionRejected {

		action =
			"purchasing.purchase_order.rejected"

		eventType =
			"vendor.purchasing.purchase_order.rejected"
	}

	if err =
		s.base.recordMutation(
			ctx,
			tx,
			actor,
			action,
			eventType,
			"vendor_purchase_order",
			order.ID,
			order.BranchID,
			order.Version,
			current,
			order,
			map[string]any{
				"purchase_order_id":       order.ID,
				"decision":                decision,
				"status":                  order.Status,
				"approval_count":          order.ApprovalCount,
				"required_approval_count": order.RequiredApprovalCount,
				"approval_generation":     order.ApprovalGeneration,
				"step_up_consumed":        stepUpConsumed,
			},
		); err != nil {

		return PurchaseOrder{}, err
	}

	if err =
		s.insertPurchaseOrderCommand(
			ctx,
			tx,
			actor,
			order,
			commandType,
			requestHash,
		); err != nil {

		return PurchaseOrder{}, err
	}

	if err =
		tx.Commit(
			ctx,
		); err != nil {

		return PurchaseOrder{}, err
	}

	return order, nil
}

func (s *PurchaseOrderService) ApprovePurchaseOrder(
	ctx context.Context,
	actor Actor,
	purchaseOrderID string,
	input PurchaseOrderApprovalInput,
) (
	PurchaseOrder,
	error,
) {
	return s.approveOrRejectPurchaseOrder(
		ctx,
		actor,
		purchaseOrderID,
		input,
		ApprovalDecisionApproved,
	)
}

func (s *PurchaseOrderService) RejectPurchaseOrder(
	ctx context.Context,
	actor Actor,
	purchaseOrderID string,
	input PurchaseOrderApprovalInput,
) (
	PurchaseOrder,
	error,
) {
	return s.approveOrRejectPurchaseOrder(
		ctx,
		actor,
		purchaseOrderID,
		input,
		ApprovalDecisionRejected,
	)
}

func (s *PurchaseOrderService) simpleTransition(
	ctx context.Context,
	actor Actor,
	purchaseOrderID string,
	input PurchaseOrderTransitionInput,
	commandType string,
	requiredFrom string,
	target string,
	eventType string,
) (
	PurchaseOrder,
	error,
) {
	scope, err :=
		s.writeScope(
			ctx,
			actor,
			PermissionManage,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	requestHash, err :=
		purchaseOrderCommandHash(
			struct {
				PurchaseOrderID string `json:"purchase_order_id"`
				ExpectedVersion int64  `json:"expected_version"`
				CommandType     string `json:"command_type"`
				Note            string `json:"note"`
			}{
				PurchaseOrderID: strings.TrimSpace(
					purchaseOrderID,
				),

				ExpectedVersion: input.ExpectedVersion,

				CommandType: commandType,

				Note: strings.TrimSpace(
					input.Note,
				),
			},
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	tx, err :=
		begin(
			ctx,
			s.base.db,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if replay,
		ok,
		replayErr :=
		s.replayPurchaseOrderCommand(
			ctx,
			tx,
			actor,
			purchaseOrderID,
			commandType,
			requestHash,
		); replayErr != nil {

		return PurchaseOrder{},
			replayErr

	} else if ok {
		if err =
			tx.Commit(
				ctx,
			); err != nil {

			return PurchaseOrder{}, err
		}

		return replay, nil
	}

	current, err :=
		s.loadPurchaseOrder(
			ctx,
			tx,
			actor.VendorID,
			purchaseOrderID,
			true,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	if err =
		requireBranch(
			scope,
			current.BranchID,
		); err != nil {

		return PurchaseOrder{}, err
	}

	if input.ExpectedVersion <= 0 ||
		current.Version !=
			input.ExpectedVersion {

		return PurchaseOrder{},
			versionConflict()
	}

	if current.Status != requiredFrom {
		return PurchaseOrder{},
			apperrors.Conflict(
				"Purchase order is not in the required lifecycle state.",
				nil,
			)
	}

	now :=
		time.Now().
			UTC()

	query :=
		`UPDATE vendor_purchase_orders
            SET status=$3,
                version=version+1,
                updated_at=$4`

	switch target {
	case PurchaseOrderOrdered:
		query +=
			`,
            ordered_by_staff_id=$5,
            ordered_at=$4`

	case PurchaseOrderClosed:
		query +=
			`,
            closed_by_staff_id=$5,
            closed_at=$4`
	}

	query +=
		`
          WHERE business_id=$1
            AND id=$2
          RETURNING ` +
			purchaseOrderColumns

	order, err :=
		scanPurchaseOrder(
			tx.QueryRow(
				ctx,
				query,
				current.BusinessID,
				current.ID,
				target,
				now,
				actor.StaffID,
			),
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	order, err =
		s.hydratePurchaseOrder(
			ctx,
			tx,
			order,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	if err =
		s.appendStatusHistory(
			ctx,
			tx,
			actor,
			order,
			current.Status,
			order.Status,
			input.Note,
		); err != nil {

		return PurchaseOrder{}, err
	}

	if err =
		s.base.recordMutation(
			ctx,
			tx,
			actor,
			"purchasing.purchase_order."+
				strings.ToLower(
					target,
				),
			eventType,
			"vendor_purchase_order",
			order.ID,
			order.BranchID,
			order.Version,
			current,
			order,
			map[string]any{
				"purchase_order_id": order.ID,
				"status":            order.Status,
			},
		); err != nil {

		return PurchaseOrder{}, err
	}

	if err =
		s.insertPurchaseOrderCommand(
			ctx,
			tx,
			actor,
			order,
			commandType,
			requestHash,
		); err != nil {

		return PurchaseOrder{}, err
	}

	if err =
		tx.Commit(
			ctx,
		); err != nil {

		return PurchaseOrder{}, err
	}

	return order, nil
}

func (s *PurchaseOrderService) MarkPurchaseOrderOrdered(
	ctx context.Context,
	actor Actor,
	purchaseOrderID string,
	input PurchaseOrderTransitionInput,
) (
	PurchaseOrder,
	error,
) {
	return s.simpleTransition(
		ctx,
		actor,
		purchaseOrderID,
		input,
		"ORDER",
		PurchaseOrderApproved,
		PurchaseOrderOrdered,
		"vendor.purchasing.purchase_order.ordered",
	)
}

func (s *PurchaseOrderService) ClosePurchaseOrder(
	ctx context.Context,
	actor Actor,
	purchaseOrderID string,
	input PurchaseOrderTransitionInput,
) (
	PurchaseOrder,
	error,
) {
	return s.simpleTransition(
		ctx,
		actor,
		purchaseOrderID,
		input,
		"CLOSE",
		PurchaseOrderReceived,
		PurchaseOrderClosed,
		"vendor.purchasing.purchase_order.closed",
	)
}

func (s *PurchaseOrderService) CancelPurchaseOrder(
	ctx context.Context,
	actor Actor,
	purchaseOrderID string,
	input PurchaseOrderTransitionInput,
) (
	PurchaseOrder,
	error,
) {
	scope, err :=
		s.writeScope(
			ctx,
			actor,
			PermissionManage,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	requestHash, err :=
		purchaseOrderCommandHash(
			struct {
				PurchaseOrderID string `json:"purchase_order_id"`
				ExpectedVersion int64  `json:"expected_version"`
				Note            string `json:"note"`
			}{
				PurchaseOrderID: strings.TrimSpace(
					purchaseOrderID,
				),

				ExpectedVersion: input.ExpectedVersion,

				Note: strings.TrimSpace(
					input.Note,
				),
			},
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	tx, err :=
		begin(
			ctx,
			s.base.db,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if replay,
		ok,
		replayErr :=
		s.replayPurchaseOrderCommand(
			ctx,
			tx,
			actor,
			purchaseOrderID,
			"CANCEL",
			requestHash,
		); replayErr != nil {

		return PurchaseOrder{},
			replayErr

	} else if ok {
		if err =
			tx.Commit(
				ctx,
			); err != nil {

			return PurchaseOrder{}, err
		}

		return replay, nil
	}

	current, err :=
		s.loadPurchaseOrder(
			ctx,
			tx,
			actor.VendorID,
			purchaseOrderID,
			true,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	if err =
		requireBranch(
			scope,
			current.BranchID,
		); err != nil {

		return PurchaseOrder{}, err
	}

	if input.ExpectedVersion <= 0 ||
		current.Version !=
			input.ExpectedVersion {

		return PurchaseOrder{},
			versionConflict()
	}

	switch current.Status {
	case PurchaseOrderDraft,
		PurchaseOrderSubmitted,
		PurchaseOrderApproved,
		PurchaseOrderOrdered:

	default:
		return PurchaseOrder{},
			apperrors.Conflict(
				"This purchase order can no longer be cancelled.",
				nil,
			)
	}

	var received int64

	if err =
		tx.QueryRow(
			ctx,
			`SELECT COALESCE(
                sum(received_purchase_quantity),
                0
            )
               FROM vendor_purchase_order_lines
              WHERE business_id=$1
                AND purchase_order_id=$2`,
			current.BusinessID,
			current.ID,
		).Scan(
			&received,
		); err != nil {

		return PurchaseOrder{}, err
	}

	if received > 0 {
		return PurchaseOrder{},
			apperrors.Conflict(
				"A purchase order with received goods cannot be cancelled.",
				nil,
			)
	}

	now :=
		time.Now().
			UTC()

	order, err :=
		scanPurchaseOrder(
			tx.QueryRow(
				ctx,
				`UPDATE vendor_purchase_orders
                    SET status='CANCELLED',
                        cancelled_by_staff_id=$3,
                        cancelled_at=$4,
                        version=version+1,
                        updated_at=$4
                  WHERE business_id=$1
                    AND id=$2
                  RETURNING `+
					purchaseOrderColumns,
				current.BusinessID,
				current.ID,
				actor.StaffID,
				now,
			),
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	order, err =
		s.hydratePurchaseOrder(
			ctx,
			tx,
			order,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	if err =
		s.appendStatusHistory(
			ctx,
			tx,
			actor,
			order,
			current.Status,
			order.Status,
			input.Note,
		); err != nil {

		return PurchaseOrder{}, err
	}

	if err =
		s.base.recordMutation(
			ctx,
			tx,
			actor,
			"purchasing.purchase_order.cancelled",
			"vendor.purchasing.purchase_order.cancelled",
			"vendor_purchase_order",
			order.ID,
			order.BranchID,
			order.Version,
			current,
			order,
			map[string]any{
				"purchase_order_id": order.ID,
				"status":            order.Status,
			},
		); err != nil {

		return PurchaseOrder{}, err
	}

	if err =
		s.insertPurchaseOrderCommand(
			ctx,
			tx,
			actor,
			order,
			"CANCEL",
			requestHash,
		); err != nil {

		return PurchaseOrder{}, err
	}

	if err =
		tx.Commit(
			ctx,
		); err != nil {

		return PurchaseOrder{}, err
	}

	return order, nil
}

func (s *PurchaseOrderService) ListPurchaseOrderApprovals(
	ctx context.Context,
	actor Actor,
	purchaseOrderID string,
) (
	[]PurchaseOrderApproval,
	error,
) {
	scope, err :=
		s.readScope(
			ctx,
			actor,
			PermissionRead,
		)

	if err != nil {
		return nil, err
	}

	order, err :=
		s.loadPurchaseOrder(
			ctx,
			s.base.db,
			actor.VendorID,
			purchaseOrderID,
			false,
		)

	if err != nil {
		return nil, err
	}

	if err =
		requireBranch(
			scope,
			order.BranchID,
		); err != nil {

		return nil, err
	}

	rows, err :=
		s.base.db.Query(
			ctx,
			`SELECT `+
				purchaseApprovalColumns+
				`
               FROM vendor_purchase_order_approvals
              WHERE business_id=$1
                AND purchase_order_id=$2
              ORDER BY
                approval_generation,
                created_at,
                id`,
			actor.VendorID,
			order.ID,
		)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	out :=
		make(
			[]PurchaseOrderApproval,
			0,
		)

	for rows.Next() {
		item, scanErr :=
			scanPurchaseApproval(
				rows,
			)

		if scanErr != nil {
			return nil, scanErr
		}

		out =
			append(
				out,
				item,
			)
	}

	return out,
		rows.Err()
}

// ApplyReceivingProgressTx is the Section 15C seam.
//
// It owns only PurchaseOrder received-progress state. It does NOT create a
// goods receipt, Inventory lot, Inventory movement or Supplier invoice.
//
// Section 15C must call this in the same PostgreSQL transaction as its durable
// receiving record after Inventory posting is valid.
//
// Quantities are absolute totals, not deltas, so receipt replay cannot
// double-increment a PurchaseOrder line.
func (s *PurchaseOrderService) ApplyReceivingProgressTx(
	ctx context.Context,
	tx pgx.Tx,
	actor Actor,
	purchaseOrderID string,
	progress []ReceivingProgressLine,
) (
	PurchaseOrder,
	error,
) {
	if tx == nil {
		return PurchaseOrder{},
			errors.New(
				"purchasing receiving progress requires transaction",
			)
	}

	scope, err :=
		s.writeScope(
			ctx,
			actor,
			PermissionManage,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	current, err :=
		s.loadPurchaseOrder(
			ctx,
			tx,
			actor.VendorID,
			purchaseOrderID,
			true,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	if err =
		requireBranch(
			scope,
			current.BranchID,
		); err != nil {

		return PurchaseOrder{}, err
	}

	switch current.Status {
	case PurchaseOrderOrdered,
		PurchaseOrderPartiallyReceived:

	default:
		return PurchaseOrder{},
			apperrors.Conflict(
				"Purchase order is not open for receiving.",
				nil,
			)
	}

	rows, err :=
		tx.Query(
			ctx,
			`SELECT
                id,
                purchase_quantity,
                received_purchase_quantity
               FROM vendor_purchase_order_lines
              WHERE business_id=$1
                AND purchase_order_id=$2
              ORDER BY id
              FOR UPDATE`,
			current.BusinessID,
			current.ID,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	type quantities struct {
		ordered  int64
		received int64
	}

	values :=
		map[string]quantities{}

	for rows.Next() {
		var (
			id       string
			ordered  int64
			received int64
		)

		if err =
			rows.Scan(
				&id,
				&ordered,
				&received,
			); err != nil {

			rows.Close()

			return PurchaseOrder{}, err
		}

		values[id] =
			quantities{
				ordered:  ordered,
				received: received,
			}
	}

	rows.Close()

	changed :=
		false

	seen :=
		map[string]bool{}

	for _, item := range progress {

		lineID :=
			strings.TrimSpace(
				item.PurchaseOrderLineID,
			)

		value, exists :=
			values[lineID]

		if !exists ||
			seen[lineID] {

			return PurchaseOrder{},
				apperrors.Conflict(
					"Receiving progress references an invalid PurchaseOrder line.",
					nil,
				)
		}

		seen[lineID] = true

		if item.ReceivedPurchaseQuantity <
			value.received ||
			item.ReceivedPurchaseQuantity >
				value.ordered {

			return PurchaseOrder{},
				apperrors.Conflict(
					"Receiving progress cannot decrease or exceed ordered quantity.",
					nil,
				)
		}

		if item.ReceivedPurchaseQuantity ==
			value.received {

			continue
		}

		changed = true

		value.received =
			item.ReceivedPurchaseQuantity

		values[lineID] = value

		if _, err =
			tx.Exec(
				ctx,
				`UPDATE vendor_purchase_order_lines
                    SET received_purchase_quantity=$4,
                        version=version+1,
                        updated_at=now()
                  WHERE business_id=$1
                    AND purchase_order_id=$2
                    AND id=$3`,
				current.BusinessID,
				current.ID,
				lineID,
				item.ReceivedPurchaseQuantity,
			); err != nil {

			return PurchaseOrder{}, err
		}
	}

	if !changed {
		return s.hydratePurchaseOrder(
			ctx,
			tx,
			current,
		)
	}

	allReceived :=
		true

	anyReceived :=
		false

	for _, value := range values {

		if value.received > 0 {
			anyReceived = true
		}

		if value.received <
			value.ordered {

			allReceived = false
		}
	}

	nextStatus :=
		PurchaseOrderOrdered

	var receivedAt any

	switch {
	case allReceived &&
		len(values) > 0:

		nextStatus =
			PurchaseOrderReceived

		receivedAt =
			time.Now().
				UTC()

	case anyReceived:
		nextStatus =
			PurchaseOrderPartiallyReceived
	}

	order, err :=
		scanPurchaseOrder(
			tx.QueryRow(
				ctx,
				`UPDATE vendor_purchase_orders
                    SET status=$3,
                        received_at=
                            CASE
                                WHEN $3='RECEIVED'
                                THEN $4::timestamptz
                                ELSE received_at
                            END,
                        version=version+1,
                        updated_at=now()
                  WHERE business_id=$1
                    AND id=$2
                  RETURNING `+
					purchaseOrderColumns,
				current.BusinessID,
				current.ID,
				nextStatus,
				receivedAt,
			),
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	order, err =
		s.hydratePurchaseOrder(
			ctx,
			tx,
			order,
		)

	if err != nil {
		return PurchaseOrder{}, err
	}

	if order.Status !=
		current.Status {

		if err =
			s.appendStatusHistory(
				ctx,
				tx,
				actor,
				order,
				current.Status,
				order.Status,
				"Goods receiving progress.",
			); err != nil {

			return PurchaseOrder{}, err
		}
	}

	if err =
		s.base.recordMutation(
			ctx,
			tx,
			actor,
			"purchasing.purchase_order.receiving_progress",
			"vendor.purchasing.purchase_order.receiving_progress",
			"vendor_purchase_order",
			order.ID,
			order.BranchID,
			order.Version,
			current,
			order,
			map[string]any{
				"purchase_order_id": order.ID,
				"status":            order.Status,
			},
		); err != nil {

		return PurchaseOrder{}, err
	}

	return order, nil
}

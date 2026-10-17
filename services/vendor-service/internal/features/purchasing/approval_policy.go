package purchasing

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"
)

type PurchaseOrderService struct {
	base   *Service
	stepUp StepUpConsumer
}

func NewPurchaseOrderService(
	base *Service,
	stepUp StepUpConsumer,
) *PurchaseOrderService {
	return &PurchaseOrderService{
		base:   base,
		stepUp: stepUp,
	}
}

func (s *PurchaseOrderService) DBReady() bool {
	return s != nil &&
		s.base != nil &&
		s.base.db != nil &&
		s.base.enabled
}

func (s *PurchaseOrderService) readScope(
	ctx context.Context,
	actor Actor,
	permission string,
) (
	Scope,
	error,
) {
	if !s.DBReady() ||
		!actor.Validate() {

		return Scope{},
			apperrors.Forbidden(
				"Purchasing access is not permitted.",
				nil,
			)
	}

	return s.base.ResolveScope(
		ctx,
		actor,
		permission,
	)
}

func (s *PurchaseOrderService) writeScope(
	ctx context.Context,
	actor Actor,
	permission string,
) (
	Scope,
	error,
) {
	if s == nil ||
		s.base == nil {

		return Scope{},
			apperrors.Forbidden(
				"Purchasing access is not permitted.",
				nil,
			)
	}

	if err := s.base.requireWrite(); err != nil {
		return Scope{}, err
	}

	return s.readScope(
		ctx,
		actor,
		permission,
	)
}

func allowedApprovalPermission(
	value string,
) bool {
	switch strings.TrimSpace(value) {
	case PermissionApprove,
		PermissionApproveBusiness,
		PermissionApproveHighValue:

		return true

	default:
		return false
	}
}

type approvalRange struct {
	min int64
	max *int64
}

func rangesOverlap(
	left approvalRange,
	right approvalRange,
) bool {
	if left.max == nil {
		return true
	}

	return right.min < *left.max
}

func validateApprovalRules(
	rules []ApprovalRuleInput,
) error {
	if len(rules) == 0 {
		return validation(
			"rules",
			"Configure at least one purchase approval rule.",
		)
	}

	if len(rules) > 100 {
		return validation(
			"rules",
			"Too many approval rules.",
		)
	}

	grouped :=
		map[string][]approvalRange{}

	for index := range rules {
		rule := &rules[index]

		if rule.BranchID != nil {
			value :=
				strings.TrimSpace(
					*rule.BranchID,
				)

			if value == "" {
				rule.BranchID = nil
			} else {
				rule.BranchID = &value
			}
		}

		if rule.MinimumAmountMinor < 0 {
			return validation(
				"rules.minimum_amount_minor",
				"Minimum amount cannot be negative.",
			)
		}

		if rule.MaximumAmountMinor != nil &&
			*rule.MaximumAmountMinor <=
				rule.MinimumAmountMinor {

			return validation(
				"rules.maximum_amount_minor",
				"Maximum amount must be greater than minimum amount.",
			)
		}

		if rule.RequiredApprovalCount < 1 ||
			rule.RequiredApprovalCount > 10 {

			return validation(
				"rules.required_approval_count",
				"Approval count must be between 1 and 10.",
			)
		}

		rule.ApprovalScope =
			strings.ToUpper(
				strings.TrimSpace(
					rule.ApprovalScope,
				),
			)

		switch rule.ApprovalScope {
		case ApprovalScopeBranchOrBusiness,
			ApprovalScopeBusinessOnly:
		default:
			return validation(
				"rules.approval_scope",
				"Use BRANCH_OR_BUSINESS or BUSINESS_ONLY.",
			)
		}

		rule.RequiredPermissionCode =
			strings.TrimSpace(
				rule.RequiredPermissionCode,
			)

		if !allowedApprovalPermission(
			rule.RequiredPermissionCode,
		) {
			return validation(
				"rules.required_permission_code",
				"Choose a supported purchasing approval permission.",
			)
		}

		key := "*"

		if rule.BranchID != nil {
			key = *rule.BranchID
		}

		grouped[key] =
			append(
				grouped[key],
				approvalRange{
					min: rule.MinimumAmountMinor,
					max: rule.MaximumAmountMinor,
				},
			)
	}

	for _, ranges := range grouped {

		sort.Slice(
			ranges,
			func(i, j int) bool {
				return ranges[i].min <
					ranges[j].min
			},
		)

		for index := 1; index < len(ranges); index++ {

			if rangesOverlap(
				ranges[index-1],
				ranges[index],
			) {
				return validation(
					"rules",
					"Approval amount ranges cannot overlap within the same branch scope.",
				)
			}
		}
	}

	return nil
}

func (s *PurchaseOrderService) validateRuleBranches(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	rules []ApprovalRuleInput,
) error {
	seen :=
		map[string]bool{}

	for _, rule := range rules {
		if rule.BranchID == nil {
			continue
		}

		branchID :=
			strings.TrimSpace(
				*rule.BranchID,
			)

		if seen[branchID] {
			continue
		}

		seen[branchID] = true

		var exists bool

		if err :=
			tx.QueryRow(
				ctx,
				`SELECT EXISTS(
                    SELECT 1
                      FROM vendor_branches
                     WHERE id=$1
                       AND business_id=$2
                       AND status='ACTIVE'
                )`,
				branchID,
				businessID,
			).Scan(
				&exists,
			); err != nil {

			return err
		}

		if !exists {
			return validation(
				"rules.branch_id",
				"Approval rule branch must belong to this Vendor.",
			)
		}
	}

	return nil
}

const approvalPolicyColumns = `
    id,
    business_id,
    name,
    allow_self_approval,
    status,
    version,
    created_at,
    updated_at
`

func scanApprovalPolicy(
	row rowScanner,
) (
	ApprovalPolicy,
	error,
) {
	var out ApprovalPolicy

	err :=
		row.Scan(
			&out.ID,
			&out.BusinessID,
			&out.Name,
			&out.AllowSelfApproval,
			&out.Status,
			&out.Version,
			&out.CreatedAt,
			&out.UpdatedAt,
		)

	return out, err
}

const approvalRuleColumns = `
    id,
    business_id,
    policy_id,
    branch_id,
    minimum_amount_minor,
    maximum_amount_minor,
    required_approval_count,
    approval_scope,
    required_permission_code,
    step_up_required,
    position,
    policy_version,
    status,
    version,
    created_at,
    updated_at
`

func scanApprovalRule(
	row rowScanner,
) (
	ApprovalRule,
	error,
) {
	var out ApprovalRule

	err :=
		row.Scan(
			&out.ID,
			&out.BusinessID,
			&out.PolicyID,
			&out.BranchID,
			&out.MinimumAmountMinor,
			&out.MaximumAmountMinor,
			&out.RequiredApprovalCount,
			&out.ApprovalScope,
			&out.RequiredPermissionCode,
			&out.StepUpRequired,
			&out.Position,
			&out.PolicyVersion,
			&out.Status,
			&out.Version,
			&out.CreatedAt,
			&out.UpdatedAt,
		)

	return out, err
}

func (s *PurchaseOrderService) loadPolicyRules(
	ctx context.Context,
	q purchaseQuerier,
	policyID string,
	businessID string,
	activeOnly bool,
) (
	[]ApprovalRule,
	error,
) {
	query :=
		`SELECT ` +
			approvalRuleColumns +
			`
           FROM vendor_purchase_approval_rules
          WHERE business_id=$1
            AND policy_id=$2`

	if activeOnly {
		query +=
			` AND status='ACTIVE'`
	}

	query +=
		` ORDER BY
            CASE WHEN branch_id IS NULL THEN 1 ELSE 0 END,
            branch_id,
            minimum_amount_minor,
            position,
            id`

	rows, err :=
		q.Query(
			ctx,
			query,
			businessID,
			policyID,
		)

	if err != nil {
		return nil, err
	}

	defer rows.Close()

	out :=
		make(
			[]ApprovalRule,
			0,
		)

	for rows.Next() {
		rule, scanErr :=
			scanApprovalRule(
				rows,
			)

		if scanErr != nil {
			return nil, scanErr
		}

		out =
			append(
				out,
				rule,
			)
	}

	return out,
		rows.Err()
}

func (s *PurchaseOrderService) GetApprovalPolicy(
	ctx context.Context,
	actor Actor,
) (
	ApprovalPolicy,
	error,
) {
	scope, err :=
		s.readScope(
			ctx,
			actor,
			PermissionRead,
		)

	if err != nil {
		return ApprovalPolicy{}, err
	}

	policy, err :=
		scanApprovalPolicy(
			s.base.db.QueryRow(
				ctx,
				`SELECT `+
					approvalPolicyColumns+
					`
                   FROM vendor_purchase_approval_policies
                  WHERE business_id=$1
                    AND status='ACTIVE'`,
				actor.VendorID,
			),
		)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		return ApprovalPolicy{},
			notFound(
				"Purchase approval policy",
			)
	}

	if err != nil {
		return ApprovalPolicy{}, err
	}

	rules, err :=
		s.loadPolicyRules(
			ctx,
			s.base.db,
			policy.ID,
			actor.VendorID,
			true,
		)

	if err != nil {
		return ApprovalPolicy{}, err
	}

	if !scope.Global {
		filtered :=
			make(
				[]ApprovalRule,
				0,
				len(rules),
			)

		for _, rule := range rules {
			if rule.BranchID == nil ||
				scope.Covers(
					*rule.BranchID,
				) {

				filtered =
					append(
						filtered,
						rule,
					)
			}
		}

		rules = filtered
	}

	policy.Rules = rules
	policy.RulesComplete = scope.Global

	return policy, nil
}

func (s *PurchaseOrderService) ReplaceApprovalPolicy(
	ctx context.Context,
	actor Actor,
	input ReplaceApprovalPolicyInput,
) (
	ApprovalPolicy,
	error,
) {
	scope, err :=
		s.writeScope(
			ctx,
			actor,
			PermissionManage,
		)

	if err != nil {
		return ApprovalPolicy{}, err
	}

	if err =
		requireGlobal(
			scope,
		); err != nil {

		return ApprovalPolicy{}, err
	}

	input.Name =
		strings.TrimSpace(
			input.Name,
		)

	if input.Name == "" ||
		len(input.Name) > 160 {

		return ApprovalPolicy{},
			validation(
				"name",
				"Provide an approval policy name.",
			)
	}

	if err =
		validateApprovalRules(
			input.Rules,
		); err != nil {

		return ApprovalPolicy{}, err
	}

	tx, err :=
		begin(
			ctx,
			s.base.db,
		)

	if err != nil {
		return ApprovalPolicy{}, err
	}

	defer func() {
		_ = tx.Rollback(ctx)
	}()

	if err =
		s.validateRuleBranches(
			ctx,
			tx,
			actor.VendorID,
			input.Rules,
		); err != nil {

		return ApprovalPolicy{}, err
	}

	current, currentErr :=
		scanApprovalPolicy(
			tx.QueryRow(
				ctx,
				`SELECT `+
					approvalPolicyColumns+
					`
                   FROM vendor_purchase_approval_policies
                  WHERE business_id=$1
                    AND status='ACTIVE'
                  FOR UPDATE`,
				actor.VendorID,
			),
		)

	var out ApprovalPolicy
	var before any

	switch {
	case errors.Is(
		currentErr,
		pgx.ErrNoRows,
	):
		if input.ExpectedVersion != 0 {
			return ApprovalPolicy{},
				versionConflict()
		}

		out, err =
			scanApprovalPolicy(
				tx.QueryRow(
					ctx,
					`INSERT INTO vendor_purchase_approval_policies(
                        id,
                        business_id,
                        name,
                        allow_self_approval
                    )
                    VALUES($1,$2,$3,$4)
                    RETURNING `+
						approvalPolicyColumns,
					"vpap_"+
						uuid.NewString(),
					actor.VendorID,
					input.Name,
					input.AllowSelfApproval,
				),
			)

		before = nil

	case currentErr != nil:
		return ApprovalPolicy{}, currentErr

	default:
		if input.ExpectedVersion <= 0 ||
			input.ExpectedVersion !=
				current.Version {

			return ApprovalPolicy{},
				versionConflict()
		}

		before = current

		if _, err =
			tx.Exec(
				ctx,
				`UPDATE vendor_purchase_approval_rules
                    SET status='ARCHIVED',
                        version=version+1,
                        updated_at=now()
                  WHERE business_id=$1
                    AND policy_id=$2
                    AND status='ACTIVE'`,
				actor.VendorID,
				current.ID,
			); err != nil {

			return ApprovalPolicy{}, err
		}

		out, err =
			scanApprovalPolicy(
				tx.QueryRow(
					ctx,
					`UPDATE vendor_purchase_approval_policies
                        SET name=$3,
                            allow_self_approval=$4,
                            version=version+1,
                            updated_at=now()
                      WHERE business_id=$1
                        AND id=$2
                      RETURNING `+
						approvalPolicyColumns,
					actor.VendorID,
					current.ID,
					input.Name,
					input.AllowSelfApproval,
				),
			)
	}

	if err != nil {
		return ApprovalPolicy{},
			mapWriteError(err)
	}

	for index, inputRule := range input.Rules {

		var rule ApprovalRule

		rule, err =
			scanApprovalRule(
				tx.QueryRow(
					ctx,
					`INSERT INTO vendor_purchase_approval_rules(
                        id,
                        business_id,
                        policy_id,
                        branch_id,
                        minimum_amount_minor,
                        maximum_amount_minor,
                        required_approval_count,
                        approval_scope,
                        required_permission_code,
                        step_up_required,
                        position,
                        policy_version
                    )
                    VALUES(
                        $1,$2,$3,$4,$5,$6,
                        $7,$8,$9,$10,$11,$12
                    )
                    RETURNING `+
						approvalRuleColumns,
					"vpar_"+
						uuid.NewString(),
					actor.VendorID,
					out.ID,
					inputRule.BranchID,
					inputRule.MinimumAmountMinor,
					inputRule.MaximumAmountMinor,
					inputRule.RequiredApprovalCount,
					inputRule.ApprovalScope,
					inputRule.RequiredPermissionCode,
					inputRule.StepUpRequired,
					index,
					out.Version,
				),
			)

		if err != nil {
			return ApprovalPolicy{},
				mapWriteError(err)
		}

		out.Rules =
			append(
				out.Rules,
				rule,
			)
	}

	if err =
		s.base.recordMutation(
			ctx,
			tx,
			actor,
			"purchasing.approval_policy.replaced",
			"vendor.purchasing.approval_policy.changed",
			"vendor_purchase_approval_policy",
			out.ID,
			"",
			out.Version,
			before,
			out,
			map[string]any{
				"policy_id":           out.ID,
				"policy_version":      out.Version,
				"rule_count":          len(out.Rules),
				"allow_self_approval": out.AllowSelfApproval,
			},
		); err != nil {

		return ApprovalPolicy{}, err
	}

	if err =
		tx.Commit(
			ctx,
		); err != nil {

		return ApprovalPolicy{}, err
	}

	return out, nil
}

func (s *PurchaseOrderService) effectiveApprovalRuleTx(
	ctx context.Context,
	tx pgx.Tx,
	businessID string,
	branchID string,
	amountMinor int64,
) (
	ApprovalPolicy,
	ApprovalRule,
	error,
) {
	policy, err :=
		scanApprovalPolicy(
			tx.QueryRow(
				ctx,
				`SELECT `+
					approvalPolicyColumns+
					`
                   FROM vendor_purchase_approval_policies
                  WHERE business_id=$1
                    AND status='ACTIVE'`,
				businessID,
			),
		)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		return ApprovalPolicy{},
			ApprovalRule{},
			apperrors.Conflict(
				"Set up an approval policy in Purchasing before submitting purchase orders.",
				nil,
			)
	}

	if err != nil {
		return ApprovalPolicy{},
			ApprovalRule{},
			err
	}

	rule, err :=
		scanApprovalRule(
			tx.QueryRow(
				ctx,
				`SELECT `+
					approvalRuleColumns+
					`
                   FROM vendor_purchase_approval_rules
                  WHERE business_id=$1
                    AND policy_id=$2
                    AND status='ACTIVE'
                    AND (
                        branch_id=$3
                        OR branch_id IS NULL
                    )
                    AND minimum_amount_minor <= $4
                    AND (
                        maximum_amount_minor IS NULL
                        OR $4 < maximum_amount_minor
                    )
                  ORDER BY
                    CASE
                        WHEN branch_id=$3
                        THEN 0
                        ELSE 1
                    END,
                    minimum_amount_minor DESC,
                    position,
                    id
                  LIMIT 1`,
				businessID,
				policy.ID,
				branchID,
				amountMinor,
			),
		)

	if errors.Is(
		err,
		pgx.ErrNoRows,
	) {
		return ApprovalPolicy{},
			ApprovalRule{},
			apperrors.Conflict(
				"No approval rule covers this purchase-order amount.",
				nil,
			)
	}

	return policy,
		rule,
		err
}

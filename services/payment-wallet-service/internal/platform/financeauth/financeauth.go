// Package financeauth implements finance RBAC and separation of duties.
//
// Two distinct questions are answered here, and conflating them is the classic
// financial-control failure:
//
//   - "may this actor perform this kind of operation at all?" — permissions,
//     granted through roles;
//   - "may this actor perform THIS operation?" — separation of duties, which
//     compares the acting identity against the identity that requested the
//     operation.
//
// An actor holding both the initiate and the approve permission still cannot
// approve their own request unless an explicit, versioned policy allows it.
package financeauth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/payment-wallet-service/internal/database"
)

// Finance permission codes. These mirror the seeded catalogue.
const (
	PermWalletView         = "finance.wallet.view"
	PermTransactionView    = "finance.transaction.view"
	PermSubscriptionManage = "finance.subscription.manage"
	PermPaymentConfirm     = "finance.payment.confirm"
	PermCommissionManage   = "finance.commission.manage"
	PermHoldCreate         = "finance.hold.create"
	PermHoldRelease        = "finance.hold.release"
	PermSettlementInitiate = "finance.settlement.initiate"
	PermSettlementApprove  = "finance.settlement.approve"
	PermRefundRequest      = "finance.refund.request"
	PermRefundApprove      = "finance.refund.approve"
	PermReversalPost       = "finance.reversal.post"
	PermReversalRequest    = "finance.reversal.request"
	PermReversalApprove    = "finance.reversal.approve"
	PermJobPaymentView     = "finance.job_payment.view"
	PermReconciliationRun  = "finance.reconciliation.run"
	PermStatementExport    = "finance.statement.export"
	PermWalletRestrict     = "finance.wallet.restrict"
	PermWalletSuspend      = "finance.wallet.suspend"

	// Section 13 provider sub-wallet operations. Posting an allocation and
	// reversing one are separate permissions: giving a provider money and
	// taking it back are different decisions with different accountability.
	PermProviderAllocationPost    = "finance.provider_allocation.post"
	PermProviderAllocationReverse = "finance.provider_allocation.reverse"
	PermProviderWithdrawalRequest = "finance.provider_withdrawal.request"
	PermProviderWithdrawalApprove = "finance.provider_withdrawal.approve"
	PermProviderWithdrawalView    = "finance.provider_withdrawal.view"
)

// Approval operations governed by a versioned policy.
const (
	OperationSettlementApproval = "SETTLEMENT_APPROVAL"
	OperationRefundApproval     = "REFUND_APPROVAL"
	OperationReversalApproval   = "REVERSAL_APPROVAL"
)

// Actor types.
const (
	ActorService = "SERVICE"
	ActorUser    = "USER"
)

// ErrForbidden means the actor lacks the required permission.
var ErrForbidden = errors.New("financeauth: actor is not permitted to perform this operation")

// ErrSelfApproval means the acting identity is the requesting identity and the
// governing policy forbids self-approval.
var ErrSelfApproval = errors.New("financeauth: separation of duties forbids approving your own request")

// ErrNoPolicy means no active approval policy governs the operation. Sensitive
// approvals fail closed rather than defaulting to "anything goes".
var ErrNoPolicy = errors.New("financeauth: no active approval policy for this operation")

// Actor is an authenticated caller of a financial operation.
type Actor struct {
	// Type is SERVICE for HMAC-authenticated platform services and USER for a
	// named operator.
	Type string
	// ID is the service name or the operator identity.
	ID string
	// OrganizationID scopes the actor to a tenant; empty means platform scope.
	OrganizationID string
	// CorrelationID and RequestID are carried into audit records.
	CorrelationID string
	RequestID     string
}

// Validate checks the actor is usable for an authorization decision.
func (a Actor) Validate() error {
	if a.Type != ActorService && a.Type != ActorUser {
		return fmt.Errorf("financeauth: actor type %q is invalid", a.Type)
	}
	if strings.TrimSpace(a.ID) == "" {
		return fmt.Errorf("financeauth: actor id is required")
	}
	return nil
}

// Identity is the stable string compared for separation of duties.
func (a Actor) Identity() string { return a.Type + ":" + a.ID }

// ApprovalPolicy is a versioned rule for a sensitive approval.
type ApprovalPolicy struct {
	ID                  string
	ScopeID             string
	Operation           string
	MinimumApprovals    int
	SelfApprovalAllowed bool
	PolicyVersion       int
}

// Authorizer resolves permissions and approval policies from the database.
type Authorizer struct {
	db database.Querier
}

// NewAuthorizer wires the authorizer over a pool or transaction.
func NewAuthorizer(db database.Querier) *Authorizer { return &Authorizer{db: db} }

// Permissions returns the distinct permission codes granted to the actor.
//
// Grants resolve at two scopes: a platform-scoped grant (organization_id NULL)
// applies everywhere, an organization-scoped grant only inside that tenant. A
// grant for a different tenant is never consulted.
func (a *Authorizer) Permissions(ctx context.Context, actor Actor) (map[string]bool, error) {
	if err := actor.Validate(); err != nil {
		return nil, err
	}
	rows, err := a.db.Query(ctx, `
		SELECT DISTINCT rp.permission_code
		FROM finance_role_grants g
		JOIN finance_role_permissions rp ON rp.role_id = g.role_id
		WHERE g.subject_type = $1
		  AND g.subject_id = $2
		  AND (g.organization_id IS NULL OR g.organization_id = NULLIF($3, ''))`,
		actor.Type, actor.ID, actor.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("financeauth: load permissions: %w", err)
	}
	defer rows.Close()

	granted := map[string]bool{}
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, fmt.Errorf("financeauth: scan permission: %w", err)
		}
		granted[code] = true
	}
	return granted, rows.Err()
}

// Require fails unless the actor holds every listed permission.
func (a *Authorizer) Require(ctx context.Context, actor Actor, permissions ...string) error {
	granted, err := a.Permissions(ctx, actor)
	if err != nil {
		return err
	}
	for _, permission := range permissions {
		if !granted[permission] {
			return fmt.Errorf("%w: %s", ErrForbidden, permission)
		}
	}
	return nil
}

// Grant assigns a role to a subject. Used by provisioning and by tests.
func (a *Authorizer) Grant(ctx context.Context, q database.Querier, id string,
	organizationID string, actorType string, subjectID string, roleID string, grantedBy string) error {
	if q == nil {
		q = a.db
	}
	_, err := q.Exec(ctx, `
		INSERT INTO finance_role_grants
			(id, organization_id, subject_type, subject_id, role_id, granted_by, granted_at)
		VALUES ($1, NULLIF($2, ''), $3, $4, $5, $6, now())
		ON CONFLICT DO NOTHING`,
		id, organizationID, actorType, subjectID, roleID, grantedBy)
	if err != nil {
		return fmt.Errorf("financeauth: grant role: %w", err)
	}
	return nil
}

// Revoke removes a role grant.
func (a *Authorizer) Revoke(ctx context.Context, q database.Querier,
	organizationID string, actorType string, subjectID string, roleID string) error {
	if q == nil {
		q = a.db
	}
	_, err := q.Exec(ctx, `
		DELETE FROM finance_role_grants
		WHERE COALESCE(organization_id, '') = $1 AND subject_type = $2
		  AND subject_id = $3 AND role_id = $4`,
		organizationID, actorType, subjectID, roleID)
	if err != nil {
		return fmt.Errorf("financeauth: revoke role: %w", err)
	}
	return nil
}

// ApprovalPolicyFor resolves the active policy governing operation for a
// scope. An organization-specific policy wins over the platform default; when
// neither exists the caller must fail closed.
func (a *Authorizer) ApprovalPolicyFor(ctx context.Context, organizationID string,
	operation string) (ApprovalPolicy, error) {
	scope := organizationID
	if scope == "" {
		scope = "PLATFORM"
	}
	var policy ApprovalPolicy
	err := a.db.QueryRow(ctx, `
		SELECT id, scope_id, operation, minimum_approvals, self_approval_allowed, policy_version
		FROM finance_approval_policies
		WHERE operation = $1
		  AND scope_id IN ($2, 'PLATFORM')
		  AND status = 'ACTIVE'
		  AND effective_from <= now()
		  AND (effective_to IS NULL OR effective_to > now())
		ORDER BY (scope_id = $2) DESC, policy_version DESC
		LIMIT 1`, operation, scope).
		Scan(&policy.ID, &policy.ScopeID, &policy.Operation, &policy.MinimumApprovals,
			&policy.SelfApprovalAllowed, &policy.PolicyVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return ApprovalPolicy{}, fmt.Errorf("%w: %s", ErrNoPolicy, operation)
	}
	if err != nil {
		return ApprovalPolicy{}, fmt.Errorf("financeauth: load approval policy: %w", err)
	}
	return policy, nil
}

// RequireApproval authorizes a sensitive approval end to end: the actor must
// hold the approving permission, an active policy must govern the operation,
// and — unless that policy explicitly permits it — the approver must not be
// the requester.
func (a *Authorizer) RequireApproval(ctx context.Context, actor Actor, permission string,
	operation string, requesterIdentity string) (ApprovalPolicy, error) {
	if err := a.Require(ctx, actor, permission); err != nil {
		return ApprovalPolicy{}, err
	}
	policy, err := a.ApprovalPolicyFor(ctx, actor.OrganizationID, operation)
	if err != nil {
		return ApprovalPolicy{}, err
	}
	if !policy.SelfApprovalAllowed && requesterIdentity != "" &&
		requesterIdentity == actor.Identity() {
		return ApprovalPolicy{}, ErrSelfApproval
	}
	return policy, nil
}

// SetApprovalPolicy installs a new policy version. Existing versions are
// retired rather than rewritten, so a historical decision can always be read
// back against the policy that governed it.
func (a *Authorizer) SetApprovalPolicy(ctx context.Context, q database.Querier, id string,
	scopeID string, operation string, minimumApprovals int, selfApprovalAllowed bool) error {
	if q == nil {
		q = a.db
	}
	if scopeID == "" {
		scopeID = "PLATFORM"
	}
	if minimumApprovals < 1 {
		return fmt.Errorf("financeauth: minimum approvals must be at least 1")
	}

	var nextVersion int
	if err := q.QueryRow(ctx, `
		SELECT COALESCE(max(policy_version), 0) + 1 FROM finance_approval_policies
		WHERE scope_id = $1 AND operation = $2`, scopeID, operation).Scan(&nextVersion); err != nil {
		return fmt.Errorf("financeauth: next policy version: %w", err)
	}

	if _, err := q.Exec(ctx, `
		UPDATE finance_approval_policies
		SET status = 'RETIRED', effective_to = now()
		WHERE scope_id = $1 AND operation = $2 AND status = 'ACTIVE'`,
		scopeID, operation); err != nil {
		return fmt.Errorf("financeauth: retire policy: %w", err)
	}

	if _, err := q.Exec(ctx, `
		INSERT INTO finance_approval_policies
			(id, scope_id, operation, minimum_approvals, self_approval_allowed,
			 policy_version, status, effective_from)
		VALUES ($1, $2, $3, $4, $5, $6, 'ACTIVE', $7)`,
		id, scopeID, operation, minimumApprovals, selfApprovalAllowed,
		nextVersion, time.Now().UTC()); err != nil {
		return fmt.Errorf("financeauth: insert policy: %w", err)
	}
	return nil
}

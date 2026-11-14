package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/ids"
)

const prefixPolicyVersion = "apv"

const policyColumns = `id, organization_id, COALESCE(code, ''), name, description, resource_type,
	action, scope_type, scope_id, threshold_amount_minor, threshold_currency,
	required_sequence, quorum, eligible_role_ids, requester_may_approve,
	separation_of_duties, expiry_seconds, origin, status, version, created_by,
	updated_by, idempotency_key, created_at, updated_at, archived_at`

// PolicyFilter narrows a policy listing.
type PolicyFilter struct {
	ResourceType string
	Action       string
	Status       string
	Origin       string
}

// CreatePolicy inserts an organization-authored policy at version 1.
func (r *Repository) CreatePolicy(ctx context.Context, q database.Querier,
	organizationID string, policy *models.Policy) error {
	if policy.ID == "" {
		policy.ID = ids.New(prefixPolicy)
	}
	policy.OrganizationID = organizationID
	policy.Version = 1
	policy.Status = models.PolicyStatusActive
	policy.Origin = models.OriginOrganization

	sequence, first := encodeSteps(policy.Steps)
	_, err := q.Exec(ctx, `
		INSERT INTO fleet_partner_approval_policies
			(id, organization_id, code, name, description, resource_type, action,
			 scope_type, scope_id, threshold_amount_minor, threshold_currency,
			 required_sequence, quorum, eligible_role_ids, requester_may_approve,
			 separation_of_duties, expiry_seconds, origin, status, version,
			 created_by, updated_by, idempotency_key)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
		        $16, $17, 'ORGANIZATION', 'ACTIVE', 1, $18, $18, $19)`,
		policy.ID, organizationID, policy.Code, policy.Name, policy.Description,
		policy.ResourceType, policy.Action, policy.ScopeType, policy.ScopeID,
		policy.ThresholdAmountMinor, nullableText(policy.ThresholdCurrency), sequence,
		first.Quorum, jsonArray(first.EligibleRoleIDs), policy.RequesterMayApprove,
		policy.SeparationOfDuties, policy.ExpirySeconds, policy.CreatedBy,
		policy.IdempotencyKey)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("approvals: create policy: %w", err)
	}
	return nil
}

// UpdatePolicy replaces the editable fields of an ACTIVE organization policy
// under optimistic concurrency and bumps its version.
func (r *Repository) UpdatePolicy(ctx context.Context, q database.Querier,
	organizationID string, policy *models.Policy, expectedVersion int64) error {
	sequence, first := encodeSteps(policy.Steps)
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_approval_policies SET
			name = $4, description = $5, scope_type = $6, scope_id = $7,
			threshold_amount_minor = $8, threshold_currency = $9,
			required_sequence = $10, quorum = $11, eligible_role_ids = $12,
			requester_may_approve = $13, separation_of_duties = $14,
			expiry_seconds = $15, updated_by = $16,
			version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $3
		  AND status = 'ACTIVE' AND origin = 'ORGANIZATION'`,
		organizationID, policy.ID, expectedVersion, policy.Name, policy.Description,
		policy.ScopeType, policy.ScopeID, policy.ThresholdAmountMinor,
		nullableText(policy.ThresholdCurrency), sequence, first.Quorum,
		jsonArray(first.EligibleRoleIDs), policy.RequesterMayApprove,
		policy.SeparationOfDuties, policy.ExpirySeconds, policy.UpdatedBy)
	if err != nil {
		return fmt.Errorf("approvals: update policy: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

// ArchivePolicy retires an ACTIVE organization policy.
func (r *Repository) ArchivePolicy(ctx context.Context, q database.Querier,
	organizationID string, policyID string, expectedVersion int64, actor string) error {
	tag, err := q.Exec(ctx, `
		UPDATE fleet_partner_approval_policies SET
			status = 'ARCHIVED', archived_at = now(), updated_by = $4,
			version = version + 1, updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND version = $3
		  AND status = 'ACTIVE' AND origin = 'ORGANIZATION'`,
		organizationID, policyID, expectedVersion, actor)
	if err != nil {
		return fmt.Errorf("approvals: archive policy: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrConflict
	}
	return nil
}

// GetPolicy loads one policy inside the tenant.
func (r *Repository) GetPolicy(ctx context.Context, q database.Querier,
	organizationID string, policyID string) (*models.Policy, error) {
	if q == nil {
		q = r.db
	}
	return scanPolicy(q.QueryRow(ctx, `SELECT `+policyColumns+`
		FROM fleet_partner_approval_policies
		WHERE organization_id = $1 AND id = $2`, organizationID, policyID))
}

// LockPolicy loads one policy for update.
func (r *Repository) LockPolicy(ctx context.Context, q database.Querier,
	organizationID string, policyID string) (*models.Policy, error) {
	return scanPolicy(q.QueryRow(ctx, `SELECT `+policyColumns+`
		FROM fleet_partner_approval_policies
		WHERE organization_id = $1 AND id = $2 FOR UPDATE`, organizationID, policyID))
}

// FindPolicyByIdempotencyKey resolves a retried policy creation.
func (r *Repository) FindPolicyByIdempotencyKey(ctx context.Context, q database.Querier,
	organizationID string, key string) (*models.Policy, error) {
	if q == nil {
		q = r.db
	}
	if key == "" {
		return nil, ErrNotFound
	}
	return scanPolicy(q.QueryRow(ctx, `SELECT `+policyColumns+`
		FROM fleet_partner_approval_policies
		WHERE organization_id = $1 AND idempotency_key = $2`, organizationID, key))
}

// ListPolicies returns the organization's policies.
func (r *Repository) ListPolicies(ctx context.Context, organizationID string,
	filter PolicyFilter) ([]models.Policy, error) {
	rows, err := r.db.Query(ctx, `SELECT `+policyColumns+`
		FROM fleet_partner_approval_policies
		WHERE organization_id = $1
		  AND ($2 = '' OR resource_type = $2)
		  AND ($3 = '' OR action = $3)
		  AND ($4 = '' OR status = $4)
		  AND ($5 = '' OR origin = $5)
		ORDER BY resource_type, action, created_at, id LIMIT 500`,
		organizationID, filter.ResourceType, filter.Action, filter.Status, filter.Origin)
	if err != nil {
		return nil, fmt.Errorf("approvals: list policies: %w", err)
	}
	defer rows.Close()
	policies := []models.Policy{}
	for rows.Next() {
		policy, scanErr := scanPolicyRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		policies = append(policies, *policy)
	}
	return policies, rows.Err()
}

// ActiveOrganizationPolicies returns the ACTIVE organization-authored
// policies for one resource and action, read inside the caller's transaction
// so the policy chosen and the request created commit against one snapshot.
func (r *Repository) ActiveOrganizationPolicies(ctx context.Context, q database.Querier,
	organizationID string, resourceType string, action string) ([]models.Policy, error) {
	if q == nil {
		q = r.db
	}
	rows, err := q.Query(ctx, `SELECT `+policyColumns+`
		FROM fleet_partner_approval_policies
		WHERE organization_id = $1 AND resource_type = $2 AND action = $3
		  AND status = 'ACTIVE' AND origin = 'ORGANIZATION'`,
		organizationID, resourceType, action)
	if err != nil {
		return nil, fmt.Errorf("approvals: read organization policies: %w", err)
	}
	defer rows.Close()
	policies := []models.Policy{}
	for rows.Next() {
		policy, scanErr := scanPolicyRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		policies = append(policies, *policy)
	}
	return policies, rows.Err()
}

// AppendPolicyVersion records one immutable version of a policy.
func (r *Repository) AppendPolicyVersion(ctx context.Context, q database.Querier,
	policy *models.Policy, changedBy string, reason string) error {
	rules, err := json.Marshal(policy.RulesSnapshot())
	if err != nil {
		return fmt.Errorf("approvals: encode policy rules: %w", err)
	}
	_, err = q.Exec(ctx, `
		INSERT INTO fleet_partner_approval_policy_versions
			(id, organization_id, policy_id, version, status, rules, changed_by, change_reason)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		ids.New(prefixPolicyVersion), policy.OrganizationID, policy.ID, policy.Version,
		policy.Status, rules, changedBy, reason)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrConflict
		}
		return fmt.Errorf("approvals: append policy version: %w", err)
	}
	return nil
}

// ListPolicyVersions returns a policy's version history, newest first.
func (r *Repository) ListPolicyVersions(ctx context.Context, organizationID string,
	policyID string) ([]models.PolicyVersion, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, policy_id, version, status, rules, changed_by, change_reason, created_at
		FROM fleet_partner_approval_policy_versions
		WHERE organization_id = $1 AND policy_id = $2
		ORDER BY version DESC`, organizationID, policyID)
	if err != nil {
		return nil, fmt.Errorf("approvals: list policy versions: %w", err)
	}
	defer rows.Close()
	versions := []models.PolicyVersion{}
	for rows.Next() {
		var version models.PolicyVersion
		var rules []byte
		if err := rows.Scan(&version.ID, &version.PolicyID, &version.Version,
			&version.Status, &rules, &version.ChangedBy, &version.ChangeReason,
			&version.CreatedAt); err != nil {
			return nil, fmt.Errorf("approvals: scan policy version: %w", err)
		}
		_ = json.Unmarshal(rules, &version.Rules)
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

// RoleUsable reports whether roleID is an ACTIVE role this organization may
// bind: a system role or one of its own custom roles.
func (r *Repository) RoleUsable(ctx context.Context, q database.Querier,
	organizationID string, roleID string) (bool, error) {
	if q == nil {
		q = r.db
	}
	var usable bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM fleet_partner_roles
			WHERE id = $2 AND status = 'ACTIVE'
			  AND (organization_id IS NULL OR organization_id = $1))`,
		organizationID, roleID).Scan(&usable)
	if err != nil {
		return false, fmt.Errorf("approvals: check role: %w", err)
	}
	return usable, nil
}

// BranchExists reports whether branchID is a non-archived branch of the
// organization.
func (r *Repository) BranchExists(ctx context.Context, q database.Querier,
	organizationID string, branchID string) (bool, error) {
	if q == nil {
		q = r.db
	}
	var exists bool
	err := q.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM fleet_partner_branches
			WHERE organization_id = $1 AND id = $2 AND status <> 'ARCHIVED')`,
		organizationID, branchID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("approvals: check branch: %w", err)
	}
	return exists, nil
}

func encodeSteps(steps []models.StepSpec) ([]byte, models.StepSpec) {
	normalized := make([]models.StepSpec, 0, len(steps))
	for _, step := range steps {
		roles := step.EligibleRoleIDs
		if roles == nil {
			roles = []string{}
		}
		normalized = append(normalized, models.StepSpec{Quorum: step.Quorum, EligibleRoleIDs: roles})
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		encoded = []byte("[]")
	}
	first := models.StepSpec{Quorum: 1}
	if len(normalized) > 0 {
		first = normalized[0]
	}
	return encoded, first
}

func nullableText(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func scanPolicy(row pgx.Row) (*models.Policy, error) { return scanPolicyRow(row) }

func scanPolicyRow(row rowScanner) (*models.Policy, error) {
	var policy models.Policy
	var sequence, roleIDs []byte
	var currency *string
	var quorum int
	err := row.Scan(&policy.ID, &policy.OrganizationID, &policy.Code, &policy.Name,
		&policy.Description, &policy.ResourceType, &policy.Action, &policy.ScopeType,
		&policy.ScopeID, &policy.ThresholdAmountMinor, &currency, &sequence, &quorum,
		&roleIDs, &policy.RequesterMayApprove, &policy.SeparationOfDuties,
		&policy.ExpirySeconds, &policy.Origin, &policy.Status, &policy.Version,
		&policy.CreatedBy, &policy.UpdatedBy, &policy.IdempotencyKey, &policy.CreatedAt,
		&policy.UpdatedAt, &policy.ArchivedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("approvals: scan policy: %w", err)
	}
	if currency != nil {
		policy.ThresholdCurrency = *currency
	}
	if len(sequence) > 0 {
		_ = json.Unmarshal(sequence, &policy.Steps)
	}
	// Template rows predate required_sequence: their single step is the
	// legacy quorum/eligible-role columns.
	if len(policy.Steps) == 0 {
		policy.Steps = []models.StepSpec{{Quorum: quorum, EligibleRoleIDs: decodeStrings(roleIDs)}}
	}
	return &policy, nil
}

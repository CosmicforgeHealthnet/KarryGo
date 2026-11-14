package usecases

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
)

// Approval policy events.
const (
	EventApprovalPolicyCreated  = "organization.approval_policy.created.v1"
	EventApprovalPolicyUpdated  = "organization.approval_policy.updated.v1"
	EventApprovalPolicyArchived = "organization.approval_policy.archived.v1"
)

var policyCodePattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{0,63}$`)

// PolicyInput carries the fields of an organization-authored approval policy.
// On update, nil pointers and a nil Steps slice keep the current value.
type PolicyInput struct {
	Actor Actor
	// PermittedBranchIDs is the caller's branch scope; empty means all.
	PermittedBranchIDs []string

	PolicyID             string
	Code                 string
	Name                 *string
	Description          *string
	ResourceType         string
	Action               string
	BranchID             *string
	ThresholdAmountMinor *int64
	ThresholdCurrency    *string
	// ClearThreshold removes an existing threshold on update.
	ClearThreshold      bool
	Steps               []models.StepSpec
	RequesterMayApprove *bool
	SeparationOfDuties  *bool
	ExpirySeconds       *int
	Reason              string

	ExpectedVersion int64
	IdempotencyKey  string
	CorrelationID   string
}

// PolicyResult carries a policy and whether a creation was replayed.
type PolicyResult struct {
	Policy   *models.Policy
	Replayed bool
}

func branchPermitted(permitted []string, branchID string) bool {
	if len(permitted) == 0 {
		return true
	}
	for _, candidate := range permitted {
		if candidate == branchID {
			return true
		}
	}
	return false
}

// policyVisible reports whether a caller with this branch scope may see a
// policy. Out-of-scope policies answer as missing.
func policyVisible(policy *models.Policy, permitted []string) bool {
	if policy.ScopeType != models.ScopeBranch || policy.ScopeID == nil {
		return true
	}
	return branchPermitted(permitted, *policy.ScopeID)
}

// validatePolicy normalizes and checks a complete policy. Every violation is
// reported at once.
func (s *Service) validatePolicy(ctx context.Context, q database.Querier,
	organizationID string, policy *models.Policy) error {
	var violations []apperrors.FieldViolation
	add := func(field string, message string) {
		violations = append(violations, apperrors.FieldViolation{Field: field, Message: message})
	}
	policy.Name = strings.TrimSpace(policy.Name)
	policy.Description = strings.TrimSpace(policy.Description)
	if policy.Name == "" || len(policy.Name) > 120 {
		add("name", "Required (at most 120 characters).")
	}
	if len(policy.Description) > 1000 {
		add("description", "At most 1000 characters.")
	}
	if policy.ResourceType == "" || len(policy.ResourceType) > 64 {
		add("resourceType", "Required (at most 64 characters).")
	}
	if policy.Action == "" || len(policy.Action) > 128 {
		add("action", "Required (at most 128 characters).")
	}
	if (policy.ThresholdAmountMinor == nil) != (policy.ThresholdCurrency == "") {
		add("thresholdAmountMinor", "Give both thresholdAmountMinor and thresholdCurrency, or neither.")
	}
	if policy.ThresholdAmountMinor != nil && *policy.ThresholdAmountMinor < 0 {
		add("thresholdAmountMinor", "Must be a whole, non-negative number of minor units.")
	}
	if policy.ThresholdCurrency != "" && !models.ValidCurrency(policy.ThresholdCurrency) {
		add("thresholdCurrency", "Use a three-letter ISO currency code.")
	}
	if policy.ExpirySeconds < models.MinPolicyExpirySecond || policy.ExpirySeconds > models.MaxPolicyExpirySecond {
		add("expirySeconds", "Between 300 seconds and 30 days.")
	}
	if len(policy.Steps) == 0 || len(policy.Steps) > models.MaxPolicySteps {
		add("steps", "Give between 1 and 5 approval steps.")
	}
	for index := range policy.Steps {
		step := &policy.Steps[index]
		if step.Quorum < 1 || step.Quorum > models.MaxStepQuorum {
			add("steps", "Each step needs a quorum between 1 and 20.")
		}
		seen := map[string]bool{}
		roles := make([]string, 0, len(step.EligibleRoleIDs))
		for _, roleID := range step.EligibleRoleIDs {
			roleID = strings.TrimSpace(roleID)
			if roleID == "" || seen[roleID] {
				continue
			}
			seen[roleID] = true
			usable, err := s.opts.Approvals.RoleUsable(ctx, q, organizationID, roleID)
			if err != nil {
				return err
			}
			if !usable {
				add("steps", "Unknown or archived role: "+roleID+".")
				continue
			}
			roles = append(roles, roleID)
		}
		step.EligibleRoleIDs = roles
	}
	if policy.ScopeType == models.ScopeBranch && policy.ScopeID != nil {
		exists, err := s.opts.Approvals.BranchExists(ctx, q, organizationID, *policy.ScopeID)
		if err != nil {
			return err
		}
		if !exists {
			add("branchId", "Unknown branch.")
		}
	}
	if len(violations) > 0 {
		return apperrors.Validation("Check the approval policy.", violations)
	}
	return nil
}

func (s *Service) requirePolicyManager(actor Actor) error {
	if actor.OrganizationID == "" || actor.OrganizationUserID == "" {
		return errorcodes.Forbidden(errorcodes.ActorNotAuthorized,
			"An organization user is required to manage approval policies.")
	}
	return nil
}

// CreatePolicy publishes a new organization approval policy at version 1.
func (s *Service) CreatePolicy(ctx context.Context, input PolicyInput) (*PolicyResult, error) {
	if err := s.requirePolicyManager(input.Actor); err != nil {
		return nil, err
	}
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.IdempotencyKey == "" {
		return nil, errorcodes.BadRequest(errorcodes.ValidationFailed,
			"An Idempotency-Key header is required.")
	}
	code := strings.ToUpper(strings.TrimSpace(input.Code))
	if !policyCodePattern.MatchString(code) {
		return nil, apperrors.Validation("Check the approval policy.", []apperrors.FieldViolation{
			{Field: "code", Message: "1-64 letters, digits, '-' or '_'."}})
	}
	policy := &models.Policy{
		Code:                code,
		ResourceType:        strings.TrimSpace(input.ResourceType),
		Action:              strings.TrimSpace(input.Action),
		ScopeType:           models.ScopeOrganization,
		Steps:               input.Steps,
		RequesterMayApprove: false,
		SeparationOfDuties:  true,
		ExpirySeconds:       models.DefaultPolicyExpiry,
		CreatedBy:           input.Actor.OrganizationUserID,
		UpdatedBy:           input.Actor.OrganizationUserID,
	}
	if input.Name != nil {
		policy.Name = *input.Name
	}
	if input.Description != nil {
		policy.Description = *input.Description
	}
	if input.BranchID != nil && strings.TrimSpace(*input.BranchID) != "" {
		branchID := strings.TrimSpace(*input.BranchID)
		if !branchPermitted(input.PermittedBranchIDs, branchID) {
			return nil, apperrors.NotFound("Branch not found.", nil)
		}
		policy.ScopeType = models.ScopeBranch
		policy.ScopeID = &branchID
	}
	policy.ThresholdAmountMinor = input.ThresholdAmountMinor
	if input.ThresholdCurrency != nil {
		policy.ThresholdCurrency = strings.TrimSpace(*input.ThresholdCurrency)
	}
	if input.RequesterMayApprove != nil {
		policy.RequesterMayApprove = *input.RequesterMayApprove
	}
	if input.SeparationOfDuties != nil {
		policy.SeparationOfDuties = *input.SeparationOfDuties
	}
	if input.ExpirySeconds != nil {
		policy.ExpirySeconds = *input.ExpirySeconds
	}
	key := input.IdempotencyKey
	policy.IdempotencyKey = &key

	if existing, err := s.opts.Approvals.FindPolicyByIdempotencyKey(ctx, nil,
		input.Actor.OrganizationID, key); err == nil {
		if existing.Code != code || existing.ResourceType != policy.ResourceType ||
			existing.Action != policy.Action {
			return nil, errorcodes.Conflict(errorcodes.IdempotencyConflictCode,
				"This idempotency key was already used for a different policy.")
		}
		return &PolicyResult{Policy: existing, Replayed: true}, nil
	} else if !errors.Is(err, repositories.ErrNotFound) {
		return nil, err
	}

	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := s.validatePolicy(ctx, tx, input.Actor.OrganizationID, policy); err != nil {
			return err
		}
		if err := s.opts.Approvals.CreatePolicy(ctx, tx, input.Actor.OrganizationID, policy); err != nil {
			if errors.Is(err, repositories.ErrConflict) {
				return errorcodes.Conflict(errorcodes.ApprovalConflictingDecision,
					"An active approval policy with this code already exists.")
			}
			return err
		}
		if err := s.opts.Approvals.AppendPolicyVersion(ctx, tx, policy,
			input.Actor.OrganizationUserID, input.Reason); err != nil {
			return err
		}
		return s.recordPolicyChange(ctx, tx, input, "approval_policy.created",
			EventApprovalPolicyCreated, nil, policy)
	})
	if err != nil {
		return nil, err
	}
	created, err := s.opts.Approvals.GetPolicy(ctx, nil, input.Actor.OrganizationID, policy.ID)
	if err != nil {
		return nil, err
	}
	return &PolicyResult{Policy: created}, nil
}

// UpdatePolicy publishes the next version of an organization policy.
//
// Requests already raised keep the rules frozen onto them when they were
// created, so an edit that relaxes a policy never weakens an approval in
// flight; the new version governs requests raised from now on.
func (s *Service) UpdatePolicy(ctx context.Context, input PolicyInput) (*models.Policy, error) {
	if err := s.requirePolicyManager(input.Actor); err != nil {
		return nil, err
	}
	if input.ExpectedVersion <= 0 {
		return nil, errorcodes.BadRequest(errorcodes.ValidationFailed,
			"An If-Match header carrying the current policy version is required.")
	}
	var updated *models.Policy
	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		current, err := s.lockEditablePolicy(ctx, tx, input)
		if err != nil {
			return err
		}
		next := *current
		next.Steps = append([]models.StepSpec(nil), current.Steps...)
		if input.Name != nil {
			next.Name = *input.Name
		}
		if input.Description != nil {
			next.Description = *input.Description
		}
		if input.BranchID != nil {
			branchID := strings.TrimSpace(*input.BranchID)
			if branchID == "" {
				next.ScopeType, next.ScopeID = models.ScopeOrganization, nil
			} else {
				if !branchPermitted(input.PermittedBranchIDs, branchID) {
					return apperrors.NotFound("Branch not found.", nil)
				}
				next.ScopeType, next.ScopeID = models.ScopeBranch, &branchID
			}
		}
		if input.ClearThreshold {
			next.ThresholdAmountMinor, next.ThresholdCurrency = nil, ""
		}
		if input.ThresholdAmountMinor != nil {
			next.ThresholdAmountMinor = input.ThresholdAmountMinor
		}
		if input.ThresholdCurrency != nil {
			next.ThresholdCurrency = strings.TrimSpace(*input.ThresholdCurrency)
		}
		if input.Steps != nil {
			next.Steps = input.Steps
		}
		if input.RequesterMayApprove != nil {
			next.RequesterMayApprove = *input.RequesterMayApprove
		}
		if input.SeparationOfDuties != nil {
			next.SeparationOfDuties = *input.SeparationOfDuties
		}
		if input.ExpirySeconds != nil {
			next.ExpirySeconds = *input.ExpirySeconds
		}
		next.UpdatedBy = input.Actor.OrganizationUserID
		if err := s.validatePolicy(ctx, tx, input.Actor.OrganizationID, &next); err != nil {
			return err
		}
		if err := s.opts.Approvals.UpdatePolicy(ctx, tx, input.Actor.OrganizationID,
			&next, input.ExpectedVersion); err != nil {
			return policyWriteError(err)
		}
		next.Version = current.Version + 1
		if err := s.opts.Approvals.AppendPolicyVersion(ctx, tx, &next,
			input.Actor.OrganizationUserID, input.Reason); err != nil {
			return err
		}
		updated = &next
		return s.recordPolicyChange(ctx, tx, input, "approval_policy.updated",
			EventApprovalPolicyUpdated, current, &next)
	})
	if err != nil {
		return nil, err
	}
	return s.opts.Approvals.GetPolicy(ctx, nil, input.Actor.OrganizationID, updated.ID)
}

// ArchivePolicy retires an organization policy. Open requests that pinned it
// keep the rules frozen onto them; new requests no longer select it.
func (s *Service) ArchivePolicy(ctx context.Context, input PolicyInput) (*models.Policy, error) {
	if err := s.requirePolicyManager(input.Actor); err != nil {
		return nil, err
	}
	if input.ExpectedVersion <= 0 {
		return nil, errorcodes.BadRequest(errorcodes.ValidationFailed,
			"An If-Match header carrying the current policy version is required.")
	}
	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		current, err := s.lockEditablePolicy(ctx, tx, input)
		if err != nil {
			return err
		}
		if err := s.opts.Approvals.ArchivePolicy(ctx, tx, input.Actor.OrganizationID,
			current.ID, input.ExpectedVersion, input.Actor.OrganizationUserID); err != nil {
			return policyWriteError(err)
		}
		archived := *current
		archived.Status = models.PolicyStatusArchived
		archived.Version = current.Version + 1
		if err := s.opts.Approvals.AppendPolicyVersion(ctx, tx, &archived,
			input.Actor.OrganizationUserID, input.Reason); err != nil {
			return err
		}
		return s.recordPolicyChange(ctx, tx, input, "approval_policy.archived",
			EventApprovalPolicyArchived, current, &archived)
	})
	if err != nil {
		return nil, err
	}
	return s.opts.Approvals.GetPolicy(ctx, nil, input.Actor.OrganizationID, input.PolicyID)
}

func (s *Service) lockEditablePolicy(ctx context.Context, tx pgx.Tx,
	input PolicyInput) (*models.Policy, error) {
	current, err := s.opts.Approvals.LockPolicy(ctx, tx, input.Actor.OrganizationID, input.PolicyID)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil, errorcodes.NotFound(errorcodes.ApprovalPolicyNotFound, "Approval policy not found.")
	}
	if err != nil {
		return nil, err
	}
	if !policyVisible(current, input.PermittedBranchIDs) {
		return nil, errorcodes.NotFound(errorcodes.ApprovalPolicyNotFound, "Approval policy not found.")
	}
	if current.Origin != models.OriginOrganization {
		return nil, errorcodes.Conflict(errorcodes.ApprovalPolicyNotEditable,
			"This policy is derived from the platform eligibility policy; publish an eligibility policy version to change it.")
	}
	if current.Status != models.PolicyStatusActive {
		return nil, errorcodes.Conflict(errorcodes.ApprovalPolicyNotEditable,
			"This approval policy is archived.")
	}
	if current.Version != input.ExpectedVersion {
		return nil, concurrency.AsConflict(concurrency.ErrVersionMismatch)
	}
	return current, nil
}

func policyWriteError(err error) error {
	if errors.Is(err, repositories.ErrConflict) {
		return concurrency.AsConflict(concurrency.ErrVersionMismatch)
	}
	return err
}

func (s *Service) recordPolicyChange(ctx context.Context, tx pgx.Tx, input PolicyInput,
	action string, eventType string, before *models.Policy, after *models.Policy) error {
	entry := audit.Entry{
		OrganizationID: input.Actor.OrganizationID,
		ActorType:      strings.ToLower(input.Actor.ActorType),
		ActorID:        input.Actor.ActorID,
		Action:         action,
		ResourceType:   "approval_policy",
		ResourceID:     after.ID,
		NewValues:      after.RulesSnapshot(),
		Reason:         input.Reason,
		RequestID:      input.CorrelationID,
		CorrelationID:  input.CorrelationID,
	}
	entry.NewValues["version"] = after.Version
	if before != nil {
		entry.PreviousValues = before.RulesSnapshot()
		entry.PreviousValues["version"] = before.Version
	}
	if input.IdempotencyKey != "" {
		entry.IdempotencyKeyReference = input.IdempotencyKey
	}
	if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
		return err
	}
	if s.opts.Events == nil {
		return nil
	}
	return s.opts.Events.Publish(ctx, tx, eventType, "approval_policy", after.ID,
		after.Version, after.OrganizationID, input.CorrelationID, map[string]any{
			"approvalPolicyId": after.ID,
			"organizationId":   after.OrganizationID,
			"code":             after.Code,
			"resourceType":     after.ResourceType,
			"action":           after.Action,
			"status":           after.Status,
			"version":          after.Version,
		})
}

// GetPolicy returns one policy visible to the caller.
func (s *Service) GetPolicy(ctx context.Context, organizationID string, permitted []string,
	policyID string) (*models.Policy, error) {
	policy, err := s.opts.Approvals.GetPolicy(ctx, nil, organizationID, policyID)
	if errors.Is(err, repositories.ErrNotFound) || (err == nil && !policyVisible(policy, permitted)) {
		return nil, errorcodes.NotFound(errorcodes.ApprovalPolicyNotFound, "Approval policy not found.")
	}
	return policy, err
}

// ListPolicies returns the policies visible to the caller.
func (s *Service) ListPolicies(ctx context.Context, organizationID string, permitted []string,
	filter repositories.PolicyFilter) ([]models.Policy, error) {
	policies, err := s.opts.Approvals.ListPolicies(ctx, organizationID, filter)
	if err != nil {
		return nil, err
	}
	visible := make([]models.Policy, 0, len(policies))
	for index := range policies {
		if policyVisible(&policies[index], permitted) {
			visible = append(visible, policies[index])
		}
	}
	return visible, nil
}

// PolicyVersions returns the version history of a visible policy.
func (s *Service) PolicyVersions(ctx context.Context, organizationID string, permitted []string,
	policyID string) ([]models.PolicyVersion, error) {
	if _, err := s.GetPolicy(ctx, organizationID, permitted, policyID); err != nil {
		return nil, err
	}
	return s.opts.Approvals.ListPolicyVersions(ctx, organizationID, policyID)
}

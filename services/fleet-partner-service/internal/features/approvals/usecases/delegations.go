package usecases

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/approvals/repositories"
	staffmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/staff/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
)

// Delegation events.
const (
	EventApprovalDelegationCreated = "organization.approval.delegation_created.v1"
	EventApprovalDelegationRevoked = "organization.approval.delegation_revoked.v1"
)

// backdatingTolerance absorbs client clock skew on effectiveFrom without
// allowing a delegation to be backdated over decisions already taken.
const backdatingTolerance = time.Minute

// DelegationInput creates a delegation from the actor to a delegate.
type DelegationInput struct {
	Actor Actor
	// PermittedBranchIDs is the caller's branch scope; empty means all.
	PermittedBranchIDs []string

	DelegateID    string
	ResourceType  string
	Action        string
	BranchID      string
	EffectiveFrom *time.Time
	ExpiresAt     time.Time
	Reason        string

	IdempotencyKey string
	CorrelationID  string
}

// DelegationResult carries a delegation and whether creation was replayed.
type DelegationResult struct {
	Delegation *models.Delegation
	Replayed   bool
}

// RevokeDelegationInput ends a delegation early.
type RevokeDelegationInput struct {
	Actor              Actor
	PermittedBranchIDs []string
	DelegationID       string
	ExpectedVersion    int64
	Reason             string
	// MayRevokeAny is true when the caller holds the approval-policy
	// management permission and may revoke any delegation in the
	// organization, not only their own.
	MayRevokeAny  bool
	CorrelationID string
}

func optionalText(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func delegationVisible(delegation *models.Delegation, permitted []string) bool {
	return delegation.BranchID == nil || branchPermitted(permitted, *delegation.BranchID)
}

// decidePermissionCovers reports whether the bindings granting the decide
// permission cover a branch (empty: the whole organization).
func decidePermissionCovers(bindings []staffmodels.RoleBinding, branchID string, now time.Time) bool {
	for _, binding := range bindings {
		if !binding.Active(now) {
			continue
		}
		switch binding.ScopeType {
		case staffmodels.ScopeOrganization:
			return true
		case staffmodels.ScopeBranch:
			if branchID != "" && binding.ScopeID != nil && *binding.ScopeID == branchID {
				return true
			}
		}
	}
	return false
}

// CreateDelegation lets the actor's approval authority be exercised by a
// delegate for a bounded time and scope.
//
// The rules (spec 38.2): a delegation is time bounded and scoped; nobody may
// delegate to themselves; the delegate must already hold the decide
// permission; and a delegation cannot grant more than the delegator holds —
// the delegator must hold the decide permission over the delegated branch
// scope now, and at decision time the delegate may use only the eligible
// roles the delegator still holds through their own role bindings.
func (s *Service) CreateDelegation(ctx context.Context, input DelegationInput) (*DelegationResult, error) {
	actor := input.Actor
	if actor.OrganizationID == "" || actor.OrganizationUserID == "" {
		return nil, errorcodes.Forbidden(errorcodes.ActorNotAuthorized,
			"An organization user is required to delegate approval authority.")
	}
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if input.IdempotencyKey == "" {
		return nil, errorcodes.BadRequest(errorcodes.ValidationFailed,
			"An Idempotency-Key header is required.")
	}
	now := s.opts.Now().UTC()
	from := now
	if input.EffectiveFrom != nil {
		from = input.EffectiveFrom.UTC()
	}
	delegation := &models.Delegation{
		DelegatorID:   actor.OrganizationUserID,
		DelegateID:    strings.TrimSpace(input.DelegateID),
		ResourceType:  optionalText(input.ResourceType),
		Action:        optionalText(input.Action),
		BranchID:      optionalText(input.BranchID),
		EffectiveFrom: from,
		ExpiresAt:     input.ExpiresAt.UTC(),
		Reason:        strings.TrimSpace(input.Reason),
		CreatedBy:     actor.OrganizationUserID,
	}
	key := input.IdempotencyKey
	delegation.IdempotencyKey = &key

	var violations []apperrors.FieldViolation
	add := func(field string, message string) {
		violations = append(violations, apperrors.FieldViolation{Field: field, Message: message})
	}
	switch {
	case delegation.DelegateID == "":
		add("delegateId", "Required.")
	case delegation.DelegateID == delegation.DelegatorID:
		add("delegateId", "You cannot delegate approval authority to yourself.")
	}
	if delegation.ExpiresAt.IsZero() {
		add("expiresAt", "Required.")
	} else {
		if !delegation.ExpiresAt.After(delegation.EffectiveFrom) || !delegation.ExpiresAt.After(now) {
			add("expiresAt", "Must be in the future and after effectiveFrom.")
		}
		if delegation.ExpiresAt.Sub(delegation.EffectiveFrom) > models.MaxDelegationDuration {
			add("expiresAt", "A delegation may last at most 90 days.")
		}
	}
	if delegation.EffectiveFrom.Before(now.Add(-backdatingTolerance)) {
		add("effectiveFrom", "A delegation cannot start in the past.")
	}
	if len(delegation.Reason) > 500 {
		add("reason", "At most 500 characters.")
	}
	if len(violations) > 0 {
		return nil, apperrors.Validation("Check the delegation.", violations)
	}
	if delegation.BranchID != nil && !branchPermitted(input.PermittedBranchIDs, *delegation.BranchID) {
		return nil, apperrors.NotFound("Branch not found.", nil)
	}

	if existing, err := s.opts.Approvals.FindDelegationByIdempotencyKey(ctx, nil,
		actor.OrganizationID, key); err == nil {
		if existing.DelegatorID != delegation.DelegatorID || existing.DelegateID != delegation.DelegateID ||
			!equalOptional(existing.ResourceType, delegation.ResourceType) ||
			!equalOptional(existing.Action, delegation.Action) ||
			!equalOptional(existing.BranchID, delegation.BranchID) ||
			!existing.ExpiresAt.Equal(delegation.ExpiresAt) {
			return nil, errorcodes.Conflict(errorcodes.IdempotencyConflictCode,
				"This idempotency key was already used for a different delegation.")
		}
		return &DelegationResult{Delegation: existing, Replayed: true}, nil
	} else if !errors.Is(err, repositories.ErrNotFound) {
		return nil, err
	}

	if s.opts.Permissions == nil {
		return nil, errorcodes.Forbidden(errorcodes.ApprovalDelegationInvalid,
			"Approval authority could not be established.")
	}
	branchID := ""
	if delegation.BranchID != nil {
		branchID = *delegation.BranchID
	}
	delegatorPermissions, err := s.opts.Permissions.EffectivePermissions(ctx,
		actor.OrganizationID, delegation.DelegatorID)
	if err != nil {
		return nil, err
	}
	if !decidePermissionCovers(delegatorPermissions[PermissionDecide], branchID, now) {
		return nil, errorcodes.Forbidden(errorcodes.PermissionEscalationForbidden,
			"You cannot delegate approval authority you do not hold for this scope.")
	}

	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		status, statusErr := s.opts.Approvals.MemberStatus(ctx, tx, actor.OrganizationID,
			delegation.DelegateID)
		if errors.Is(statusErr, repositories.ErrNotFound) {
			// A member of another organization answers as missing.
			return apperrors.NotFound("Delegate not found.", nil)
		}
		if statusErr != nil {
			return statusErr
		}
		if status != staffmodels.UserStatusActive {
			return errorcodes.UnprocessableEntity(errorcodes.ApprovalDelegationInvalid,
				"The delegate must be an active member of the organization.")
		}
		delegatePermissions, permErr := s.opts.Permissions.EffectivePermissions(ctx,
			actor.OrganizationID, delegation.DelegateID)
		if permErr != nil {
			return permErr
		}
		if !anyActiveBinding(delegatePermissions[PermissionDecide], now) {
			return errorcodes.UnprocessableEntity(errorcodes.ApprovalDelegationInvalid,
				"The delegate must already hold the permission to decide approvals.")
		}
		if delegation.BranchID != nil {
			exists, branchErr := s.opts.Approvals.BranchExists(ctx, tx, actor.OrganizationID,
				*delegation.BranchID)
			if branchErr != nil {
				return branchErr
			}
			if !exists {
				return apperrors.Validation("Check the delegation.", []apperrors.FieldViolation{
					{Field: "branchId", Message: "Unknown branch."}})
			}
		}
		if lockErr := s.opts.Approvals.LockDelegator(ctx, tx, actor.OrganizationID,
			delegation.DelegatorID); lockErr != nil {
			return lockErr
		}
		delegation.OrganizationID = actor.OrganizationID
		overlap, overlapErr := s.opts.Approvals.HasOverlappingDelegation(ctx, tx, delegation)
		if overlapErr != nil {
			return overlapErr
		}
		if overlap {
			return errorcodes.Conflict(errorcodes.ApprovalDelegationOverlap,
				"You already delegate this scope to this person for an overlapping period.")
		}
		if createErr := s.opts.Approvals.CreateDelegation(ctx, tx, actor.OrganizationID,
			delegation); createErr != nil {
			if errors.Is(createErr, repositories.ErrConflict) {
				return errorcodes.Conflict(errorcodes.IdempotencyConflictCode,
					"This idempotency key was already used.")
			}
			return createErr
		}
		return s.recordDelegationChange(ctx, tx, actor, input.CorrelationID,
			"approval.delegation_created", EventApprovalDelegationCreated, delegation,
			delegation.Reason, key)
	})
	if err != nil {
		return nil, err
	}
	created, err := s.opts.Approvals.GetDelegation(ctx, nil, actor.OrganizationID, delegation.ID)
	if err != nil {
		return nil, err
	}
	return &DelegationResult{Delegation: created}, nil
}

// RevokeDelegation ends a delegation before it expires. The delegator may
// revoke their own; an approval-policy manager may revoke any.
func (s *Service) RevokeDelegation(ctx context.Context, input RevokeDelegationInput) (*models.Delegation, error) {
	if input.Actor.OrganizationID == "" || input.Actor.OrganizationUserID == "" {
		return nil, errorcodes.Forbidden(errorcodes.ActorNotAuthorized,
			"An organization user is required to revoke a delegation.")
	}
	if input.ExpectedVersion <= 0 {
		return nil, errorcodes.BadRequest(errorcodes.ValidationFailed,
			"An If-Match header carrying the current delegation version is required.")
	}
	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		current, err := s.opts.Approvals.LockDelegation(ctx, tx, input.Actor.OrganizationID,
			input.DelegationID)
		if errors.Is(err, repositories.ErrNotFound) ||
			(err == nil && !delegationVisible(current, input.PermittedBranchIDs)) {
			return errorcodes.NotFound(errorcodes.ApprovalDelegationNotFound, "Delegation not found.")
		}
		if err != nil {
			return err
		}
		if current.DelegatorID != input.Actor.OrganizationUserID && !input.MayRevokeAny {
			return errorcodes.Forbidden(errorcodes.ActorNotAuthorized,
				"Only the delegator or an approval-policy manager can revoke this delegation.")
		}
		switch current.Status(s.opts.Now()) {
		case models.DelegationRevoked, models.DelegationExpired:
			return errorcodes.Conflict(errorcodes.ApprovalDelegationEnded,
				"This delegation has already ended.")
		}
		if current.Version != input.ExpectedVersion {
			return concurrency.AsConflict(concurrency.ErrVersionMismatch)
		}
		reason := strings.TrimSpace(input.Reason)
		if err := s.opts.Approvals.RevokeDelegation(ctx, tx, input.Actor.OrganizationID,
			current.ID, input.ExpectedVersion, input.Actor.OrganizationUserID, reason); err != nil {
			if errors.Is(err, repositories.ErrConflict) {
				return concurrency.AsConflict(concurrency.ErrVersionMismatch)
			}
			return err
		}
		revoked := *current
		revoked.Version++
		return s.recordDelegationChange(ctx, tx, input.Actor, input.CorrelationID,
			"approval.delegation_revoked", EventApprovalDelegationRevoked, &revoked, reason, "")
	})
	if err != nil {
		return nil, err
	}
	return s.opts.Approvals.GetDelegation(ctx, nil, input.Actor.OrganizationID, input.DelegationID)
}

func (s *Service) recordDelegationChange(ctx context.Context, tx pgx.Tx, actor Actor,
	correlationID string, action string, eventType string, delegation *models.Delegation,
	reason string, idempotencyKey string) error {
	values := map[string]any{
		"delegationId":  delegation.ID,
		"delegatorId":   delegation.DelegatorID,
		"delegateId":    delegation.DelegateID,
		"resourceType":  delegation.ResourceType,
		"action":        delegation.Action,
		"branchId":      delegation.BranchID,
		"effectiveFrom": delegation.EffectiveFrom,
		"expiresAt":     delegation.ExpiresAt,
		"version":       delegation.Version,
	}
	branchID := ""
	if delegation.BranchID != nil {
		branchID = *delegation.BranchID
	}
	if err := s.opts.Audit.Record(ctx, tx, audit.Entry{
		OrganizationID:          actor.OrganizationID,
		BranchID:                branchID,
		ActorType:               strings.ToLower(actor.ActorType),
		ActorID:                 actor.ActorID,
		Action:                  action,
		ResourceType:            "approval_delegation",
		ResourceID:              delegation.ID,
		NewValues:               values,
		Reason:                  reason,
		RequestID:               correlationID,
		CorrelationID:           correlationID,
		IdempotencyKeyReference: idempotencyKey,
	}); err != nil {
		return err
	}
	if s.opts.Events == nil {
		return nil
	}
	return s.opts.Events.Publish(ctx, tx, eventType, "approval_delegation", delegation.ID,
		delegation.Version, actor.OrganizationID, correlationID, values)
}

// GetDelegation returns one delegation visible to the caller.
func (s *Service) GetDelegation(ctx context.Context, organizationID string, permitted []string,
	delegationID string) (*models.Delegation, error) {
	delegation, err := s.opts.Approvals.GetDelegation(ctx, nil, organizationID, delegationID)
	if errors.Is(err, repositories.ErrNotFound) ||
		(err == nil && !delegationVisible(delegation, permitted)) {
		return nil, errorcodes.NotFound(errorcodes.ApprovalDelegationNotFound, "Delegation not found.")
	}
	return delegation, err
}

// ListDelegations returns the delegations visible to the caller.
func (s *Service) ListDelegations(ctx context.Context, organizationID string, permitted []string,
	filter repositories.DelegationFilter) ([]models.Delegation, error) {
	filter.Status = strings.ToUpper(strings.TrimSpace(filter.Status))
	switch filter.Status {
	case "", models.DelegationActive, models.DelegationScheduled, models.DelegationExpired,
		models.DelegationRevoked:
	default:
		return nil, apperrors.Validation("Check the filters.", []apperrors.FieldViolation{
			{Field: "status", Message: "Use ACTIVE, SCHEDULED, EXPIRED or REVOKED."}})
	}
	delegations, err := s.opts.Approvals.ListDelegations(ctx, organizationID, filter, s.opts.Now().UTC())
	if err != nil {
		return nil, err
	}
	visible := make([]models.Delegation, 0, len(delegations))
	for index := range delegations {
		if delegationVisible(&delegations[index], permitted) {
			visible = append(visible, delegations[index])
		}
	}
	return visible, nil
}

// Now exposes the service clock so views derive delegation status from the
// same instant the rules use.
func (s *Service) Now() time.Time { return s.opts.Now() }

func equalOptional(a *string, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

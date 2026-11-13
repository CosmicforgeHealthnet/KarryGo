package usecases

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/shared/go/apperrors"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/audit"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/concurrency"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// Managed-provider administration (spec Sections 10 and 39.3): edit,
// reactivate, remove, capabilities, policies and decline history. All of
// it is organization-side membership data. The provider's platform
// identity, credentials and verification stay with the owning provider
// service and are never touched here.

// liveWorkStatuses are assignment states that still hold the provider.
var liveWorkStatuses = []string{"CREATED", "PENDING_PROVIDER_RESPONSE", "ACCEPTED", "READY_FOR_EXECUTION",
	"IN_PROGRESS", "EXECUTION_START_PENDING", "RECONCILIATION_REQUIRED"}

func (s *Service) membership(ctx context.Context, organizationID string, membershipID string) (*models.Membership, error) {
	membership, err := s.opts.Providers.GetMembership(ctx, organizationID, membershipID)
	if err != nil {
		if errors.Is(err, providerrepos.ErrNotFound) {
			return nil, apperrors.NotFound("Provider not found.", nil)
		}
		return nil, err
	}
	return membership, nil
}

// LiveWorkCount reports company work still held by a membership, across
// marketplace and private assignments.
func (s *Service) LiveWorkCount(ctx context.Context, q database.Querier, organizationID string, membershipID string) (int, error) {
	var count int
	err := q.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM fleet_partner_provider_assignments
		    WHERE organization_id = $1 AND membership_id = $2 AND status = ANY($3::text[]))
		+ (SELECT count(*) FROM fleet_partner_private_assignments
		    WHERE organization_id = $1 AND provider_membership_id = $2 AND status = ANY($3::text[]))`,
		organizationID, membershipID, liveWorkStatuses).Scan(&count)
	return count, err
}

// Reactivate returns a suspended provider to ACTIVE. Activation gates
// (sub-wallet, compliance) are re-evaluated so reactivation cannot skip a
// requirement that failed while the provider was suspended.
func (s *Service) Reactivate(ctx context.Context, tenantCtx tenant.Context, membershipID string, expectedVersion int64) error {
	membership, err := s.membership(ctx, tenantCtx.OrganizationID, membershipID)
	if err != nil {
		return err
	}
	if membership.Status != models.StatusSuspended {
		return errorcodes.Conflict(errorcodes.ProviderMembershipNotActive, "Only a suspended provider can be reactivated.")
	}
	gates, _, err := s.EvaluateActivation(ctx, tenantCtx.OrganizationID, membershipID)
	if err != nil {
		return err
	}
	var failing []string
	for _, gate := range gates {
		// The pending-setup gate describes first activation; a suspended
		// membership has already been through it.
		if !gate.Passed && gate.Name != "membership_pending_setup" {
			failing = append(failing, gate.Name)
		}
	}
	if len(failing) > 0 {
		return errorcodes.Conflict(errorcodes.ProviderProvisioningIncomplete,
			"This provider cannot be reactivated until these requirements are met: "+strings.Join(failing, ", ")+".")
	}
	return s.ChangeStatus(ctx, ChangeStatusInput{Tenant: tenantCtx, MembershipID: membershipID,
		TargetStatus: models.StatusActive, ReasonCode: "reactivated_by_organization", ExpectedVersion: expectedVersion})
}

// Remove ends the membership. Work the provider still holds must be
// finished, cancelled or reassigned first: removing a provider mid-trip
// would strand a customer. The provider's platform identity is kept.
func (s *Service) Remove(ctx context.Context, tenantCtx tenant.Context, membershipID string, reasonCode string, expectedVersion int64) error {
	membership, err := s.membership(ctx, tenantCtx.OrganizationID, membershipID)
	if err != nil {
		return err
	}
	if reasonCode = strings.TrimSpace(reasonCode); reasonCode == "" {
		reasonCode = "removed_by_organization"
	}
	live, err := s.LiveWorkCount(ctx, s.opts.Pool, tenantCtx.OrganizationID, membershipID)
	if err != nil {
		return err
	}
	if live > 0 {
		return errorcodes.Conflict(errorcodes.ProviderHasConflictingWork,
			"This provider still has company work in progress. Reassign or finish it first.")
	}
	switch membership.Status {
	case models.StatusActive, models.StatusSuspended:
		if err := s.ChangeStatus(ctx, ChangeStatusInput{Tenant: tenantCtx, MembershipID: membershipID,
			TargetStatus: models.StatusEnding, ReasonCode: reasonCode, ExpectedVersion: expectedVersion}); err != nil {
			return err
		}
		fallthrough
	case models.StatusEnding:
		return s.ChangeStatus(ctx, ChangeStatusInput{Tenant: tenantCtx, MembershipID: membershipID,
			TargetStatus: models.StatusRemoved, ReasonCode: reasonCode})
	case models.StatusInvited, models.StatusPendingProviderAcceptance, models.StatusPendingSetup:
		// Not yet working for the company: the invitation is withdrawn.
		return s.ChangeStatus(ctx, ChangeStatusInput{Tenant: tenantCtx, MembershipID: membershipID,
			TargetStatus: models.StatusCancelled, ReasonCode: reasonCode, ExpectedVersion: expectedVersion})
	default:
		return errorcodes.Conflict(errorcodes.ProviderMembershipNotActive, "This provider is no longer with the organization.")
	}
}

// UpdateMembershipInput carries organization-side membership fields.
type UpdateMembershipInput struct {
	Tenant           tenant.Context
	MembershipID     string
	HomeBranchID     *string
	RelationshipType *string
	EmploymentType   *string
	EmployeeNumber   *string
	ExpectedVersion  int64
}

var relationshipTypes = map[string]bool{"EMPLOYEE": true, "CONTRACTOR": true, "PARTNER": true}
var employmentTypes = map[string]bool{"FULL_TIME": true, "PART_TIME": true, "CASUAL": true}

// UpdateMembership edits the organization's own record of the provider.
func (s *Service) UpdateMembership(ctx context.Context, input UpdateMembershipInput) (*models.Membership, error) {
	membership, err := s.membership(ctx, input.Tenant.OrganizationID, input.MembershipID)
	if err != nil {
		return nil, err
	}
	if membership.Status == models.StatusRemoved || membership.Status == models.StatusCancelled ||
		membership.Status == models.StatusDeclined || membership.Status == models.StatusExpired {
		return nil, errorcodes.Conflict(errorcodes.ProviderMembershipNotActive, "This provider is no longer with the organization.")
	}
	homeBranch, relationship, employment, employee := membership.HomeBranchID, membership.RelationshipType, membership.EmploymentType, membership.EmployeeNumber
	if input.HomeBranchID != nil {
		value := strings.TrimSpace(*input.HomeBranchID)
		var exists bool
		if err := s.opts.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM fleet_partner_branches
			WHERE organization_id = $1 AND id = $2 AND status = 'ACTIVE')`, input.Tenant.OrganizationID, value).Scan(&exists); err != nil {
			return nil, err
		}
		if !exists {
			return nil, apperrors.Validation("The selected branch is not available.",
				[]apperrors.FieldViolation{{Field: "homeBranchId", Message: "Unknown branch."}})
		}
		homeBranch = &value
	}
	if input.RelationshipType != nil {
		value := strings.ToUpper(strings.TrimSpace(*input.RelationshipType))
		if !relationshipTypes[value] {
			return nil, apperrors.Validation("Relationship must be EMPLOYEE, CONTRACTOR or PARTNER.",
				[]apperrors.FieldViolation{{Field: "relationshipType", Message: "Invalid value."}})
		}
		relationship = value
	}
	if input.EmploymentType != nil {
		value := strings.ToUpper(strings.TrimSpace(*input.EmploymentType))
		if value == "" {
			employment = nil
		} else if !employmentTypes[value] {
			return nil, apperrors.Validation("Employment type must be FULL_TIME, PART_TIME or CASUAL.",
				[]apperrors.FieldViolation{{Field: "employmentType", Message: "Invalid value."}})
		} else {
			employment = &value
		}
	}
	if input.EmployeeNumber != nil {
		value := strings.TrimSpace(*input.EmployeeNumber)
		employee = &value
	}
	expected := input.ExpectedVersion
	if expected == 0 {
		expected = membership.Version
	}
	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE fleet_partner_organization_provider_memberships SET
				home_branch_id = $4, relationship_type = $5, employment_type = $6, employee_number = $7,
				version = version + 1, updated_at = now()
			WHERE organization_id = $1 AND id = $2 AND version = $3`,
			input.Tenant.OrganizationID, input.MembershipID, expected, homeBranch, relationship, employment, employee)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return concurrency.AsConflict(concurrency.ErrVersionMismatch)
		}
		entry := audit.EntryFromTenant(input.Tenant, "provider.updated", "provider_membership", input.MembershipID)
		entry.PreviousValues = map[string]any{"homeBranchId": membership.HomeBranchID, "relationshipType": membership.RelationshipType}
		entry.NewValues = map[string]any{"homeBranchId": homeBranch, "relationshipType": relationship}
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		return s.opts.Events.Publish(ctx, tx, "organization.provider.updated.v1", "provider_membership",
			input.MembershipID, expected+1, input.Tenant.OrganizationID, input.Tenant.CorrelationID,
			map[string]any{"membershipId": input.MembershipID, "homeBranchId": homeBranch})
	})
	if err != nil {
		return nil, err
	}
	return s.membership(ctx, input.Tenant.OrganizationID, input.MembershipID)
}

// Capabilities returns the provider's active organization capabilities.
func (s *Service) Capabilities(ctx context.Context, organizationID string, membershipID string) ([]models.Capability, error) {
	if _, err := s.membership(ctx, organizationID, membershipID); err != nil {
		return nil, err
	}
	return s.opts.Providers.ListCapabilities(ctx, organizationID, membershipID)
}

// ReplaceCapabilities sets what the organization will use the provider for.
// A capability can only narrow, never widen, the provider's service
// category: a dispatch rider cannot be declared a haulage driver here.
func (s *Service) ReplaceCapabilities(ctx context.Context, tenantCtx tenant.Context, membershipID string, capabilities []models.Capability) ([]models.Capability, error) {
	membership, err := s.membership(ctx, tenantCtx.OrganizationID, membershipID)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	for index, capability := range capabilities {
		capability.ServiceType = strings.ToUpper(strings.TrimSpace(capability.ServiceType))
		if capability.ServiceType == "" {
			capability.ServiceType = membership.ServiceCategory
		}
		if capability.ServiceType != membership.ServiceCategory {
			return nil, apperrors.Validation("A capability must match the provider's service category ("+membership.ServiceCategory+").",
				[]apperrors.FieldViolation{{Field: "capabilities.serviceType", Message: "Does not match the provider's service."}})
		}
		if capability.ValidUntil != nil && !capability.ValidUntil.After(now) {
			return nil, apperrors.Validation("A capability end date must be in the future.",
				[]apperrors.FieldViolation{{Field: "capabilities.validUntil", Message: "Must be in the future."}})
		}
		capabilities[index] = capability
	}
	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := s.opts.Providers.ReplaceCapabilities(ctx, tx, tenantCtx.OrganizationID, membershipID, capabilities); err != nil {
			return err
		}
		entry := audit.EntryFromTenant(tenantCtx, "provider.capabilities_replaced", "provider_membership", membershipID)
		entry.NewValues = map[string]any{"count": len(capabilities)}
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		return s.opts.Events.Publish(ctx, tx, "organization.provider.capabilities_changed.v1", "provider_membership",
			membershipID, membership.Version, tenantCtx.OrganizationID, tenantCtx.CorrelationID,
			map[string]any{"membershipId": membershipID, "count": len(capabilities)})
	})
	if err != nil {
		return nil, err
	}
	return s.opts.Providers.ListCapabilities(ctx, tenantCtx.OrganizationID, membershipID)
}

// Policy returns the provider's assignment policy (a default when none is set).
func (s *Service) Policy(ctx context.Context, organizationID string, membershipID string) (*models.Policy, error) {
	if _, err := s.membership(ctx, organizationID, membershipID); err != nil {
		return nil, err
	}
	policy, err := s.opts.Providers.GetPolicy(ctx, organizationID, membershipID)
	if errors.Is(err, providerrepos.ErrNotFound) {
		return &models.Policy{MembershipID: membershipID, OrganizationID: organizationID, AssignmentMode: "OFFER",
			ProviderDeclineRequired: true, VisibilityPolicy: "ORGANIZATION"}, nil
	}
	return policy, err
}

var assignmentModes = map[string]bool{"OFFER": true, "BROADCAST": true, "DIRECTED_OFFER": true}

// ReplacePolicy stores the provider's assignment policy. The provider's
// right to decline is a platform invariant (spec 2.6/11) and cannot be
// switched off: a request that tries is refused rather than ignored.
func (s *Service) ReplacePolicy(ctx context.Context, tenantCtx tenant.Context, policy models.Policy, declineRequiredSet bool) (*models.Policy, error) {
	membership, err := s.membership(ctx, tenantCtx.OrganizationID, policy.MembershipID)
	if err != nil {
		return nil, err
	}
	if declineRequiredSet && !policy.ProviderDeclineRequired {
		return nil, apperrors.Validation("Providers can always decline company work; this cannot be turned off.",
			[]apperrors.FieldViolation{{Field: "providerDeclineRequired", Message: "Must be true."}})
	}
	policy.AssignmentMode = strings.ToUpper(strings.TrimSpace(policy.AssignmentMode))
	if policy.AssignmentMode == "" {
		policy.AssignmentMode = "OFFER"
	}
	policy.VisibilityPolicy = strings.ToUpper(strings.TrimSpace(policy.VisibilityPolicy))
	if policy.VisibilityPolicy == "" {
		policy.VisibilityPolicy = "ORGANIZATION"
	}
	if !assignmentModes[policy.AssignmentMode] || (policy.VisibilityPolicy != "ORGANIZATION" && policy.VisibilityPolicy != "BRANCH") {
		return nil, apperrors.Validation("Check the assignment mode and visibility.", []apperrors.FieldViolation{
			{Field: "assignmentMode", Message: "OFFER, BROADCAST or DIRECTED_OFFER."},
			{Field: "visibilityPolicy", Message: "ORGANIZATION or BRANCH."}})
	}
	policy.ProviderDeclineRequired = true
	policy.OrganizationID = tenantCtx.OrganizationID
	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		if err := s.opts.Providers.UpsertPolicy(ctx, tx, tenantCtx.OrganizationID, &policy); err != nil {
			return err
		}
		entry := audit.EntryFromTenant(tenantCtx, "provider.policy_replaced", "provider_membership", policy.MembershipID)
		entry.NewValues = map[string]any{"assignmentMode": policy.AssignmentMode, "visibilityPolicy": policy.VisibilityPolicy}
		if err := s.opts.Audit.Record(ctx, tx, entry); err != nil {
			return err
		}
		return s.opts.Events.Publish(ctx, tx, "organization.provider.policy_changed.v1", "provider_membership",
			policy.MembershipID, membership.Version, tenantCtx.OrganizationID, tenantCtx.CorrelationID,
			map[string]any{"membershipId": policy.MembershipID, "assignmentMode": policy.AssignmentMode})
	})
	if err != nil {
		return nil, err
	}
	return s.opts.Providers.GetPolicy(ctx, tenantCtx.OrganizationID, policy.MembershipID)
}

// Decline is one recorded provider decline, marketplace or private.
type Decline struct {
	AssignmentID  string    `json:"assignmentId"`
	WorkOrigin    string    `json:"workOrigin"`
	ReasonCode    string    `json:"reasonCode"`
	SafetyRelated bool      `json:"safetyRelated"`
	DeclinedAt    time.Time `json:"declinedAt"`
}

// Declines lists a provider's declines, newest first. Free-text notes are
// not returned: the reason code is what the organization needs, and a
// provider's own words about a safety concern stay with safety review.
// Declines are never a disciplinary signal on their own (spec 11, 17).
func (s *Service) Declines(ctx context.Context, organizationID string, membershipID string, limit int) ([]Decline, error) {
	if _, err := s.membership(ctx, organizationID, membershipID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.opts.Pool.Query(ctx, `
		SELECT assignment_id, 'MARKETPLACE', COALESCE(reason_code, ''), safety_related, occurred_at
		FROM fleet_partner_assignment_responses
		WHERE organization_id = $1 AND membership_id = $2 AND response = 'DECLINED'
		UNION ALL
		SELECT id, 'PRIVATE', decline_reason, false, declined_at
		FROM fleet_partner_private_assignments
		WHERE organization_id = $1 AND provider_membership_id = $2 AND declined_at IS NOT NULL
		ORDER BY 5 DESC, 1 LIMIT $3`, organizationID, membershipID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	declines := []Decline{}
	for rows.Next() {
		var decline Decline
		if err := rows.Scan(&decline.AssignmentID, &decline.WorkOrigin, &decline.ReasonCode, &decline.SafetyRelated, &decline.DeclinedAt); err != nil {
			return nil, err
		}
		upper := strings.ToUpper(decline.ReasonCode)
		if strings.Contains(upper, "UNSAFE") || strings.Contains(upper, "DEFECT") || strings.Contains(upper, "SAFETY") {
			decline.SafetyRelated = true
		}
		declines = append(declines, decline)
	}
	return declines, rows.Err()
}

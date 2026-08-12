package usecases

import (
	"context"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
)

// MembershipSummary is Section 30 WP19/WP20's compatibility surface: the
// facts an owning provider service (driver-hauling-service,
// driver-dispatch-delivery-service, driver-taxi-service) needs to enrich
// ITS OWN provider login/session response with providerMode and, when the
// provider is organization-managed, a tenant-safe organization summary and
// the membership's assignment/field-visibility policy.
//
// fleet-partner-service never issues a provider session itself -- it owns
// no provider identity or auth (see README's "what it never owns"). This
// is why the summary is exposed as an internal, service-authenticated read
// rather than a provider-facing endpoint: the OWNING service calls it,
// server-side, and splices the result into a response it already controls.
// A caller-supplied providerMode is never trusted; this summary is always
// derived fresh from the authoritative membership row.
type MembershipSummary struct {
	// ProviderMode is models.ProviderModeIndependent when no live
	// membership exists for (providerService, providerID) in
	// serviceCategory, or models.ProviderModeOrganizationManaged when one
	// does. Every field below is populated ONLY in the latter case.
	ProviderMode string

	Organization            *OrganizationSummary
	MembershipStatus        string
	MembershipRole          string // Membership.RelationshipType (EMPLOYEE/CONTRACTOR/PARTNER)
	ServiceCategory         string
	AssignmentMode          string // Policy.AssignmentMode
	ProviderDeclineRequired bool   // Policy.ProviderDeclineRequired -- always true; a platform invariant, not organization-configurable
	FieldVisibilityPolicy   string // Policy.VisibilityPolicy
}

// OrganizationSummary is the tenant-safe organization projection: never a
// customer contract price, company margin/commission, org wallet balance,
// internal finance/dispatcher note, or another provider's information (see
// Section 30 WP20/WP25). Built exclusively from OrganizationReader's own
// narrow, pre-vetted methods (OrganizationStatus, BranchName) -- never from
// the full Organization/Branch models directly -- so a field this summary
// was never given cannot leak by accident.
type OrganizationSummary struct {
	ID          string
	DisplayName string
	Status      string
	BranchID    string
	BranchName  string
	BranchType  string
}

// GetMembershipSummaryForProvider derives a provider's ProviderMode and, if
// organization-managed, its tenant-safe summary. It is authoritative and
// scoped to the provider's single ACTIVE-lifecycle (live) membership for
// serviceCategory -- see models.Membership.Live and FindLiveByProvider's
// own status filter -- so a SUSPENDED or REMOVED membership is never
// reported as if it were live, and no cross-organization data can leak (a
// provider has at most one live membership per service category).
func (s *Service) GetMembershipSummaryForProvider(ctx context.Context, providerService string, providerID string) (MembershipSummary, error) {
	membership, err := s.opts.Providers.FindLiveByProvider(ctx, nil, providerService, providerID)
	if err != nil {
		if err == providerrepos.ErrNotFound {
			summary := MembershipSummary{ProviderMode: models.ProviderModeIndependent}
			s.recordProviderContract(summary.ProviderMode)
			return summary, nil
		}
		return MembershipSummary{}, err
	}
	if membership == nil {
		summary := MembershipSummary{ProviderMode: models.ProviderModeIndependent}
		s.recordProviderContract(summary.ProviderMode)
		return summary, nil
	}

	summary := MembershipSummary{
		ProviderMode:     models.ProviderModeOrganizationManaged,
		MembershipStatus: membership.Status,
		MembershipRole:   membership.RelationshipType,
		ServiceCategory:  membership.ServiceCategory,
	}

	orgName, orgStatus, err := s.opts.Organizations.OrganizationStatus(ctx, membership.OrganizationID)
	if err != nil {
		return MembershipSummary{}, err
	}
	org := &OrganizationSummary{
		ID:          membership.OrganizationID,
		DisplayName: orgName,
		Status:      orgStatus,
	}
	if membership.HomeBranchID != nil && *membership.HomeBranchID != "" {
		branchName, branchType, err := s.opts.Organizations.BranchName(ctx, membership.OrganizationID, *membership.HomeBranchID)
		if err == nil {
			// Branch lookup is best-effort: a stale/archived branch
			// reference must not fail the whole summary (an owning
			// service's login flow), only omit the branch fields.
			org.BranchID = *membership.HomeBranchID
			org.BranchName = branchName
			org.BranchType = branchType
		}
	}
	summary.Organization = org

	policy, err := s.opts.Providers.GetPolicy(ctx, membership.OrganizationID, membership.ID)
	if err == nil && policy != nil {
		// Policy lookup is best-effort for the same reason as branch above
		// -- a membership predating policy provisioning (should not
		// happen in practice, but defensive) still returns a usable
		// summary with the mode/organization facts intact.
		summary.AssignmentMode = policy.AssignmentMode
		summary.ProviderDeclineRequired = policy.ProviderDeclineRequired
		summary.FieldVisibilityPolicy = policy.VisibilityPolicy
	}

	s.recordProviderContract(summary.ProviderMode)
	return summary, nil
}

func (s *Service) recordProviderContract(providerMode string) {
	// Both labels have a fixed, bounded vocabulary. Provider, organization,
	// request and membership identifiers are deliberately never labels.
	s.opts.Metrics.IncCounter("fleet_partner_provider_contract_responses_total", map[string]string{
		"contract_version": "1",
		"provider_mode":    providerMode,
	}, 1)
}

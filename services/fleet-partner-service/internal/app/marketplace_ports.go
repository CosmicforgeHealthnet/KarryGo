package app

import (
	"context"
	"errors"

	providermodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"

	marketplaceusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// membershipReaderAdapter lets the marketplace feature read managed provider
// memberships without importing the providers feature's repository package.
//
// This adapter lives at the composition root because it is the only place
// allowed to know both features. Its real job is error translation: without
// it, a membership that does not exist would surface as an opaque internal
// error instead of a clean "provider not eligible" decision, because the
// marketplace code cannot match on the providers package's sentinel.
type membershipReaderAdapter struct {
	repo *providerrepos.Repository
}

// GetMembership implements marketplaceusecases.MembershipReader.
func (a membershipReaderAdapter) GetMembership(ctx context.Context, organizationID string, membershipID string) (*providermodels.Membership, error) {
	membership, err := a.repo.GetMembership(ctx, organizationID, membershipID)
	if err != nil {
		if errors.Is(err, providerrepos.ErrNotFound) {
			return nil, marketplaceusecases.ErrMembershipNotFound
		}
		return nil, err
	}
	return membership, nil
}

// NewMembershipReader builds the marketplace membership port.
//
// Exported because the composition root that assembles the marketplace service
// lives in package main. It was unexported — and unused — for long enough that
// every deployment ran with a nil eligibility checker, which is why an
// acknowledgement or acceptance from a suspended provider had nothing to
// consult.
func NewMembershipReader(repo *providerrepos.Repository) marketplaceusecases.MembershipReader {
	return membershipReaderAdapter{repo: repo}
}

// Compile-time proof that the adapter satisfies the port.
var _ marketplaceusecases.MembershipReader = membershipReaderAdapter{}

// candidateMembershipListerAdapter lets the marketplace feature's automatic
// reassignment coordinator list active provider memberships without
// importing the providers feature's repository package, mirroring
// membershipReaderAdapter above.
type candidateMembershipListerAdapter struct {
	repo *providerrepos.Repository
}

// ListActiveMemberships implements marketplaceusecases.CandidateMembershipLister.
//
// This performs no scoring or matching -- it returns active memberships in
// the order the providers repository already returns them (most recently
// created first), excluding the given set. The actual eligibility decision
// for each candidate is made entirely by the marketplace domain's own
// Reassign use case; this adapter's only job is supplying candidates to try.
func (a candidateMembershipListerAdapter) ListActiveMemberships(ctx context.Context, organizationID string, excluding []string, limit int) ([]string, error) {
	excluded := make(map[string]bool, len(excluding))
	for _, id := range excluding {
		excluded[id] = true
	}
	memberships, err := a.repo.ListMemberships(ctx, tenant.Query{OrganizationID: organizationID, Limit: 100})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, limit)
	for _, membership := range memberships {
		if membership.Status != providermodels.StatusActive {
			continue
		}
		if excluded[membership.ID] {
			continue
		}
		out = append(out, membership.ID)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// NewCandidateMembershipLister builds the marketplace candidate-membership
// port. Exported for the same reason as NewMembershipReader: the
// composition root is the one place allowed to know both features.
func NewCandidateMembershipLister(repo *providerrepos.Repository) marketplaceusecases.CandidateMembershipLister {
	return candidateMembershipListerAdapter{repo: repo}
}

var _ marketplaceusecases.CandidateMembershipLister = candidateMembershipListerAdapter{}

// membershipHistoryReaderAdapter lets the marketplace feature list a
// provider's own membership history without importing the providers
// feature's repository package, mirroring membershipReaderAdapter above.
type membershipHistoryReaderAdapter struct {
	repo *providerrepos.Repository
}

// ListByProvider implements marketplaceusecases.MembershipHistoryReader.
func (a membershipHistoryReaderAdapter) ListByProvider(ctx context.Context, providerService string,
	providerID string) ([]providermodels.Membership, error) {
	return a.repo.ListMembershipsByProvider(ctx, providerService, providerID)
}

// NewMembershipHistoryReader builds the marketplace membership-history port.
func NewMembershipHistoryReader(repo *providerrepos.Repository) marketplaceusecases.MembershipHistoryReader {
	return membershipHistoryReaderAdapter{repo: repo}
}

var _ marketplaceusecases.MembershipHistoryReader = membershipHistoryReaderAdapter{}

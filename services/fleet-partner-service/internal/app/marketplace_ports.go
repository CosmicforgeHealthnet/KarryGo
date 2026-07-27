package app

import (
	"context"
	"errors"

	providermodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/models"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"

	marketplaceusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/usecases"
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

// newMembershipReader builds the marketplace membership port.
func newMembershipReader(repo *providerrepos.Repository) marketplaceusecases.MembershipReader {
	return membershipReaderAdapter{repo: repo}
}

// Compile-time proof that the adapter satisfies the port.
var _ marketplaceusecases.MembershipReader = membershipReaderAdapter{}

package app

import (
	"context"
	"errors"

	documentusecases "cosmicforge/logistics/services/fleet-partner-service/internal/features/documents/usecases"
	providerrepos "cosmicforge/logistics/services/fleet-partner-service/internal/features/providers/repositories"
)

// documentsMembershipAdapter lets the documents feature resolve a provider
// identity to its live fleet membership without importing the providers
// feature's repository package.
//
// This adapter lives at the composition root because it is the only place
// allowed to know both features — the same reasoning as membershipReaderAdapter
// in marketplace_ports.go, for a different consumer of the same repository
// method.
type documentsMembershipAdapter struct {
	repo *providerrepos.Repository
}

// FindLiveMembership implements documentusecases.MembershipLookup.
func (a documentsMembershipAdapter) FindLiveMembership(ctx context.Context, providerService string,
	providerID string) (organizationID string, membershipID string, err error) {
	membership, err := a.repo.FindLiveByProvider(ctx, nil, providerService, providerID)
	if err != nil {
		if errors.Is(err, providerrepos.ErrNotFound) {
			return "", "", documentusecases.ErrMembershipNotFound
		}
		return "", "", err
	}
	if membership == nil || !membership.Live() {
		return "", "", documentusecases.ErrMembershipNotFound
	}
	return membership.OrganizationID, membership.ID, nil
}

// NewDocumentsMembershipLookup builds the documents membership port.
func NewDocumentsMembershipLookup(repo *providerrepos.Repository) documentusecases.MembershipLookup {
	return documentsMembershipAdapter{repo: repo}
}

// Compile-time proof that the adapter satisfies the port.
var _ documentusecases.MembershipLookup = documentsMembershipAdapter{}

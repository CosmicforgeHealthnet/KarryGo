package usecases

import (
	"context"
	"time"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
)

// ProviderMembershipView is the provider-safe view of one of their own
// organization memberships: no company-internal notes, no other providers'
// data, and — matching the same "safe fields only" rule
// ProviderAssignmentSummaryView already follows — no financial fields at
// all, since a membership record carries none to begin with.
type ProviderMembershipView struct {
	MembershipID     string     `json:"membershipId"`
	OrganizationID   string     `json:"organizationId"`
	ServiceCategory  string     `json:"serviceCategory"`
	RelationshipType string     `json:"relationshipType"`
	Status           string     `json:"status"`
	JoinedAt         *time.Time `json:"joinedAt,omitempty"`
	SuspendedAt      *time.Time `json:"suspendedAt,omitempty"`
	EndingAt         *time.Time `json:"endingAt,omitempty"`
	RemovedAt        *time.Time `json:"removedAt,omitempty"`
}

// ListProviderMemberships returns every membership -- live or historical --
// this provider identity has ever held, across every organization. This is
// the provider-self "membership history" read (Section 28/34): a provider's
// own decline/acceptance/assignment history is already served by
// ListProviderOffers/ListProviderAssignments; this is the remaining piece,
// which of those organizations the provider has actually belonged to and
// when.
func (s *Service) ListProviderMemberships(ctx context.Context, providerService string,
	providerID string) ([]ProviderMembershipView, error) {
	if s.opts.MembershipHistory == nil {
		return nil, errorcodes.Unavailable(errorcodes.FleetServiceUnavailable, "Membership history is not available.")
	}
	memberships, err := s.opts.MembershipHistory.ListByProvider(ctx, providerService, providerID)
	if err != nil {
		return nil, err
	}
	views := make([]ProviderMembershipView, 0, len(memberships))
	for _, m := range memberships {
		views = append(views, ProviderMembershipView{
			MembershipID: m.ID, OrganizationID: m.OrganizationID, ServiceCategory: m.ServiceCategory,
			RelationshipType: m.RelationshipType, Status: m.Status,
			JoinedAt: m.JoinedAt, SuspendedAt: m.SuspendedAt, EndingAt: m.EndingAt, RemovedAt: m.RemovedAt,
		})
	}
	return views, nil
}

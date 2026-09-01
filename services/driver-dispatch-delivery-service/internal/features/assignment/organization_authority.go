package assignment

import (
	"context"
	"errors"
	"strings"

	"cosmicforge/logistics/shared/go/providercompat"
)

// FleetOrganizationAuthority reuses Fleet Partner's HMAC-protected,
// tenant-safe membership summary. Membership never substitutes for Dispatch's
// provider, vehicle, restriction, availability, or active-job validation.
type FleetOrganizationAuthority struct{ reader providercompat.Reader }

func NewFleetOrganizationAuthority(reader providercompat.Reader) *FleetOrganizationAuthority {
	return &FleetOrganizationAuthority{reader: reader}
}

func (a *FleetOrganizationAuthority) ValidateMembership(ctx context.Context, organizationID, providerID string) error {
	if a == nil || a.reader == nil || strings.TrimSpace(organizationID) == "" || strings.TrimSpace(providerID) == "" {
		return errors.New("organization authority unavailable")
	}
	summary, err := a.reader.ReadMembershipSummary(ctx, "driver-dispatch-delivery-service", providerID)
	if err != nil {
		return err
	}
	if summary.ProviderMode != providercompat.ModeOrganizationManaged || summary.Organization == nil || summary.Membership == nil || summary.AssignmentPolicy == nil || summary.Organization.ID != organizationID || !strings.EqualFold(summary.Organization.Status, "ACTIVE") || !strings.EqualFold(summary.Membership.Status, "ACTIVE") || !strings.EqualFold(summary.Membership.ServiceCategory, "DISPATCH") || !strings.EqualFold(summary.AssignmentPolicy.AssignmentMode, "OFFER") || !summary.AssignmentPolicy.ProviderDeclineRequired {
		return ErrConflict
	}
	return nil
}

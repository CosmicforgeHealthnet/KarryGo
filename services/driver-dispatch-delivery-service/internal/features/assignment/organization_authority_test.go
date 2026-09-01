package assignment

import (
	"context"
	"testing"

	"cosmicforge/logistics/shared/go/providercompat"
)

type membershipReaderFake struct {
	summary providercompat.Summary
	err     error
}

func (f membershipReaderFake) ReadMembershipSummary(context.Context, string, string) (providercompat.Summary, error) {
	return f.summary, f.err
}

func TestFleetOrganizationAuthorityRequiresExactActiveDispatchMembership(t *testing.T) {
	valid := providercompat.Summary{ProviderMode: providercompat.ModeOrganizationManaged, Organization: &providercompat.Organization{ID: "org-1", Status: "ACTIVE"}, Membership: &providercompat.Membership{Status: "ACTIVE", ServiceCategory: "DISPATCH"}, AssignmentPolicy: &providercompat.AssignmentPolicy{AssignmentMode: "OFFER", ProviderDeclineRequired: true}}
	a := NewFleetOrganizationAuthority(membershipReaderFake{summary: valid})
	if err := a.ValidateMembership(context.Background(), "org-1", "provider-1"); err != nil {
		t.Fatal(err)
	}
	if err := a.ValidateMembership(context.Background(), "org-2", "provider-1"); err == nil {
		t.Fatal("foreign organization accepted")
	}
	valid.Membership.Status = "SUSPENDED"
	if err := NewFleetOrganizationAuthority(membershipReaderFake{summary: valid}).ValidateMembership(context.Background(), "org-1", "provider-1"); err == nil {
		t.Fatal("suspended membership accepted")
	}
}

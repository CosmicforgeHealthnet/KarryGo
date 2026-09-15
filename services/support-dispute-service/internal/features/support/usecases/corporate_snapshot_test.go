package supportusecases

import (
	"context"
	"testing"

	supportclients "cosmicforge/logistics/services/support-dispute-service/internal/features/support/clients"
	supportmodels "cosmicforge/logistics/services/support-dispute-service/internal/features/support/models"
)

// stubCorporate answers with a fixed snapshot, or refuses.
type stubCorporate struct {
	snapshot supportclients.CorporateSnapshot
	calls    int
}

func (s *stubCorporate) ResolveOrganization(
	context.Context, supportmodels.ComplainantType, string,
) supportclients.CorporateSnapshot {
	s.calls++
	return s.snapshot
}

func corporateService(t *testing.T, corporate supportclients.OrganizationResolver) (*SupportService, *memRepo) {
	t.Helper()
	repo := newMemRepo()
	return NewSupportService(repo, Options{Corporate: corporate}), repo
}

// A managed provider's case must carry the organization it happened under.
// An operator handling the incident needs to know who to call.
func TestProviderCaseSnapshotsTheOrganization(t *testing.T) {
	corporate := &stubCorporate{snapshot: supportclients.CorporateSnapshot{
		OrganizationReference: "org_123", MembershipID: "mem_1", Resolved: true}}
	service, _ := corporateService(t, corporate)

	complaint, err := service.CreateComplaint(context.Background(), CreateComplaintInput{
		ComplainantType: supportmodels.ComplainantHaulingProvider,
		ComplainantID:   "prov_1",
		ServiceType:     supportmodels.ServiceTypeHauling,
		Subject:         "Cargo damaged in transit",
		Description:     "The pallet was crushed before pickup.",
	})
	if err != nil {
		t.Fatalf("create complaint: %v", err)
	}
	if complaint.OrganizationReference == nil || *complaint.OrganizationReference != "org_123" {
		t.Fatalf("organization was not snapshotted: %+v", complaint.OrganizationReference)
	}
}

// An SOS raised while a provider holds goods must be created even if Fleet is
// unreachable. The incident is the urgent thing; the organization label is a
// reporting dimension, and trading the first for the second would be wrong
// every single time.
func TestSOSSucceedsWhenFleetIsUnavailable(t *testing.T) {
	corporate := &stubCorporate{snapshot: supportclients.CorporateSnapshot{}}
	service, _ := corporateService(t, corporate)

	complaint, err := service.CreateSOS(context.Background(), SOSInput{
		ComplainantType: supportmodels.ComplainantDispatchProvider,
		ComplainantID:   "rider_1",
		ServiceType:     supportmodels.ServiceTypeDispatch,
		Description:     "Threatened while holding a package.",
	})
	if err != nil {
		t.Fatalf("SOS must not fail because Fleet could not be read: %v", err)
	}
	if complaint.Priority != supportmodels.PriorityEmergency {
		t.Fatalf("priority = %q, want emergency", complaint.Priority)
	}
	if complaint.OrganizationReference != nil {
		t.Fatalf("an unresolved organization must be absent, not guessed: %v", *complaint.OrganizationReference)
	}
	if corporate.calls != 1 {
		t.Fatalf("the resolver was called %d times, want exactly 1", corporate.calls)
	}
}

// An independent provider genuinely has no organization. Recording one would
// be a fabrication, not a default.
func TestIndependentProviderGetsNoOrganization(t *testing.T) {
	service, _ := corporateService(t, &stubCorporate{snapshot: supportclients.CorporateSnapshot{}})

	complaint, err := service.CreateComplaint(context.Background(), CreateComplaintInput{
		ComplainantType: supportmodels.ComplainantTaxiProvider,
		ComplainantID:   "driver_1",
		ServiceType:     supportmodels.ServiceTypeTaxi,
		Subject:         "Passenger dispute",
		Description:     "Passenger refused to pay the fare.",
	})
	if err != nil {
		t.Fatalf("create complaint: %v", err)
	}
	if complaint.OrganizationReference != nil {
		t.Fatalf("an independent provider was given an organization: %v", *complaint.OrganizationReference)
	}
}

// A nil resolver is a supported configuration, not a crash: the platform ran
// without this dimension until now and must keep running without it.
func TestUnconfiguredResolverStillCreatesCases(t *testing.T) {
	service, _ := corporateService(t, nil)
	if _, err := service.CreateSOS(context.Background(), SOSInput{
		ComplainantType: supportmodels.ComplainantHaulingProvider,
		ComplainantID:   "prov_9",
		ServiceType:     supportmodels.ServiceTypeHauling,
	}); err != nil {
		t.Fatalf("SOS failed with no corporate resolver configured: %v", err)
	}
}

// A typed-nil Fleet resolver reaches the service as a non-nil interface, so
// the nil-receiver guard is what actually keeps an unconfigured deployment
// from panicking on its first support case.
func TestTypedNilFleetResolverDoesNotPanic(t *testing.T) {
	var resolver *supportclients.FleetOrganizationResolver
	service, _ := corporateService(t, resolver)
	if _, err := service.CreateComplaint(context.Background(), CreateComplaintInput{
		ComplainantType: supportmodels.ComplainantHaulingProvider,
		ComplainantID:   "prov_2",
		ServiceType:     supportmodels.ServiceTypeHauling,
		Subject:         "Late pickup",
		Description:     "Waited two hours at the depot.",
	}); err != nil {
		t.Fatalf("create complaint: %v", err)
	}
}

// A customer has no corporate context, so Fleet must not be consulted at all.
func TestCustomerCaseDoesNotConsultFleet(t *testing.T) {
	got := supportclients.NoopOrganizationResolver{}.ResolveOrganization(
		context.Background(), supportmodels.ComplainantCustomer, "cust_1")
	if got.Resolved {
		t.Fatal("the no-op resolver reported a resolved organization")
	}
}

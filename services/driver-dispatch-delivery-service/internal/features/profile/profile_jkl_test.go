package profile

import (
	"context"
	"encoding/json"
	"testing"

	authclients "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/clients"
)

const (
	testUUIDProvider = "11111111-1111-1111-1111-111111111111"
	testUUIDBooking  = "22222222-2222-2222-2222-222222222222"
	testUUIDCustomer = "33333333-3333-3333-3333-333333333333"
)

func TestProfileUpdatedEventUsesRequestCorrelationID(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	publisher := &fakeProfileEventPublisher{}
	service := NewServiceWithEvents(repo, publisher)

	city := "Lekki"
	if _, err := service.UpdateMe(context.Background(), AuthContext{
		ProviderID: "provider-123", PhoneNumber: "+2348000000001", CorrelationID: "patch-req-123",
	}, UpdateProviderInput{City: &city}); err != nil {
		t.Fatalf("UpdateMe() error = %v", err)
	}
	if len(publisher.profileUpdated) != 1 {
		t.Fatalf("profile.updated events = %d, want 1", len(publisher.profileUpdated))
	}
	ev := publisher.profileUpdated[0]
	if ev.CorrelationID != "patch-req-123" {
		t.Fatalf("correlation_id = %q, want patch-req-123", ev.CorrelationID)
	}
	if len(ev.ChangedFields) != 1 || ev.ChangedFields[0] != "city" {
		t.Fatalf("changed_fields = %v, want [city]", ev.ChangedFields)
	}
}

func TestOnboardingCompletedPayloadAndExactlyOnce(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	publisher := &fakeProfileEventPublisher{}
	service := NewServiceWithEvents(repo, publisher)
	auth := AuthContext{ProviderID: "provider-123", PhoneNumber: "+2348000000001", CorrelationID: "complete-req-123"}

	if _, err := service.Onboarding(context.Background(), auth, OnboardingInput{
		FullName: "Ada Lovelace", State: "Lagos", City: "Ikeja", OperationType: OperationIndividual,
	}); err != nil {
		t.Fatalf("Onboarding() error = %v", err)
	}
	if len(publisher.onboardingCompleted) != 0 {
		t.Fatalf("event fired after onboarding alone: %d", len(publisher.onboardingCompleted))
	}
	if _, err := service.SetEmergencyContact(context.Background(), auth, EmergencyContactInput{
		FullName: "Grace Hopper", Phone: "+2348012345678", Relationship: "Sister",
	}); err != nil {
		t.Fatalf("SetEmergencyContact() error = %v", err)
	}
	if len(publisher.onboardingCompleted) != 0 {
		t.Fatalf("event fired after profile + emergency contact only: %d", len(publisher.onboardingCompleted))
	}
	if _, err := service.SetGuarantor(context.Background(), auth, GuarantorInput{
		FullName: "Alan Turing", Phone: "+2348099999999",
	}); err != nil {
		t.Fatalf("SetGuarantor() error = %v", err)
	}
	if len(publisher.onboardingCompleted) != 1 {
		t.Fatalf("event count = %d, want 1", len(publisher.onboardingCompleted))
	}
	ev := publisher.onboardingCompleted[0]
	if ev.CorrelationID != "complete-req-123" || ev.ProviderID != "provider-123" || ev.Phone != "+2348000000001" || ev.OperationType != "individual" {
		t.Fatalf("onboarding event payload = %+v", ev)
	}

	if _, err := service.SetEmergencyContact(context.Background(), auth, EmergencyContactInput{
		FullName: "Katherine Johnson", Phone: "+2348012345679", Relationship: "Aunt",
	}); err != nil {
		t.Fatalf("second SetEmergencyContact() error = %v", err)
	}
	if _, err := service.SetGuarantor(context.Background(), auth, GuarantorInput{
		FullName: "Mary Jackson", Phone: "+2348099999998",
	}); err != nil {
		t.Fatalf("second SetGuarantor() error = %v", err)
	}
	city := "Yaba"
	if _, err := service.UpdateMe(context.Background(), auth, UpdateProviderInput{City: &city}); err != nil {
		t.Fatalf("UpdateMe after completion error = %v", err)
	}
	if len(publisher.onboardingCompleted) != 1 {
		t.Fatalf("event republished after completion: %d", len(publisher.onboardingCompleted))
	}
	provider, _, _ := repo.GetProviderByID(context.Background(), "provider-123")
	if !provider.OnboardingComplete {
		t.Fatal("onboarding_complete = false, want true")
	}
}

func TestOnboardingCompletedDoesNotFireForProfileAndGuarantorOnly(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	publisher := &fakeProfileEventPublisher{}
	service := NewServiceWithEvents(repo, publisher)
	auth := AuthContext{ProviderID: "provider-123", PhoneNumber: "+2348000000001"}

	if _, err := service.Onboarding(context.Background(), auth, OnboardingInput{
		FullName: "Ada Lovelace", State: "Lagos", City: "Ikeja", OperationType: OperationFleet,
	}); err != nil {
		t.Fatalf("Onboarding() error = %v", err)
	}
	if _, err := service.SetGuarantor(context.Background(), auth, GuarantorInput{
		FullName: "Alan Turing", Phone: "+2348099999999",
	}); err != nil {
		t.Fatalf("SetGuarantor() error = %v", err)
	}
	if len(publisher.onboardingCompleted) != 0 {
		t.Fatalf("event fired without emergency contact: %d", len(publisher.onboardingCompleted))
	}
}

func TestSubscriberSessionCreatedCreatesSparseProviderAndDuplicateIsSafe(t *testing.T) {
	repo := newFakeProfileRepository()
	payload, _ := json.Marshal(authclients.SessionCreatedEvent{
		ProviderID: testUUIDProvider, PhoneNumber: "+2348012345678",
	})
	if err := HandleSessionCreatedPayload(context.Background(), repo, payload); err != nil {
		t.Fatalf("HandleSessionCreatedPayload() error = %v", err)
	}
	if err := HandleSessionCreatedPayload(context.Background(), repo, payload); err != nil {
		t.Fatalf("duplicate HandleSessionCreatedPayload() error = %v", err)
	}
	provider, ok, _ := repo.GetProviderByID(context.Background(), testUUIDProvider)
	if !ok || provider.Phone != "+2348012345678" {
		t.Fatalf("provider = %+v ok=%v", provider, ok)
	}
}

func TestSubscriberVerificationStatusUpdated(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure(testUUIDProvider, "+2348012345678")
	payload, _ := json.Marshal(VerificationStatusUpdatedEvent{
		ProviderID: testUUIDProvider, VerificationStatus: StatusVerified,
	})
	if err := HandleVerificationStatusUpdatedPayload(context.Background(), repo, payload); err != nil {
		t.Fatalf("verified payload error = %v", err)
	}
	provider, _, _ := repo.GetProviderByID(context.Background(), testUUIDProvider)
	if provider.VerificationStatus != StatusVerified || !provider.IsActive {
		t.Fatalf("provider after verified = %+v", provider)
	}

	payload, _ = json.Marshal(VerificationStatusUpdatedEvent{
		ProviderID: testUUIDProvider, VerificationStatus: StatusSuspended,
	})
	if err := HandleVerificationStatusUpdatedPayload(context.Background(), repo, payload); err != nil {
		t.Fatalf("suspended payload error = %v", err)
	}
	provider, _, _ = repo.GetProviderByID(context.Background(), testUUIDProvider)
	if provider.VerificationStatus != StatusSuspended || provider.IsActive {
		t.Fatalf("provider after suspended = %+v", provider)
	}
}

func TestSubscriberTripCompletedIncrementsTotalTrips(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure(testUUIDProvider, "+2348012345678")
	payload, _ := json.Marshal(TripCompletedEvent{ProviderID: testUUIDProvider})
	if err := HandleTripCompletedPayload(context.Background(), repo, payload); err != nil {
		t.Fatalf("HandleTripCompletedPayload() error = %v", err)
	}
	provider, _, _ := repo.GetProviderByID(context.Background(), testUUIDProvider)
	if provider.TotalTrips != 1 {
		t.Fatalf("total_trips = %d, want 1", provider.TotalTrips)
	}
}

func TestSubscriberCustomerRatingSubmittedInsertsRecalculatesAndDeduplicates(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure(testUUIDProvider, "+2348012345678")
	comment := "Great ride"
	payload, _ := json.Marshal(CustomerRatingSubmittedEvent{
		ProviderID: testUUIDProvider, BookingID: testUUIDBooking, RatedByCustomerID: testUUIDCustomer, Score: 5, Comment: &comment,
	})
	if err := HandleCustomerRatingSubmittedPayload(context.Background(), repo, payload); err != nil {
		t.Fatalf("HandleCustomerRatingSubmittedPayload() error = %v", err)
	}
	if err := HandleCustomerRatingSubmittedPayload(context.Background(), repo, payload); err != nil {
		t.Fatalf("duplicate HandleCustomerRatingSubmittedPayload() error = %v", err)
	}
	if len(repo.ratings) != 1 {
		t.Fatalf("ratings len = %d, want 1", len(repo.ratings))
	}
	provider, _, _ := repo.GetProviderByID(context.Background(), testUUIDProvider)
	if provider.AvgRating != 5 {
		t.Fatalf("avg_rating = %f, want 5", provider.AvgRating)
	}
}

func TestSubscriberBadPayloadsReturnErrors(t *testing.T) {
	repo := newFakeProfileRepository()
	cases := []struct {
		name string
		run  func() error
	}{
		{name: "session", run: func() error { return HandleSessionCreatedPayload(context.Background(), repo, []byte(`{`)) }},
		{name: "verification", run: func() error {
			return HandleVerificationStatusUpdatedPayload(context.Background(), repo, []byte(`{"provider_id":"bad","verification_status":"verified"}`))
		}},
		{name: "trip", run: func() error {
			return HandleTripCompletedPayload(context.Background(), repo, []byte(`{"provider_id":"bad"}`))
		}},
		{name: "rating", run: func() error {
			return HandleCustomerRatingSubmittedPayload(context.Background(), repo, []byte(`{"provider_id":"`+testUUIDProvider+`","booking_id":"`+testUUIDBooking+`","rated_by_customer_id":"`+testUUIDCustomer+`","score":9}`))
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); err == nil {
				t.Fatal("expected error for bad payload")
			}
		})
	}
}

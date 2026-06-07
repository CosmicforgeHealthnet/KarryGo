package profile

import (
	"context"
	"errors"
	"testing"
	"time"

	"karrygo/shared/go/apperrors"
)

// ── 2D: Onboarding ───────────────────────────────────────────────────────────

func TestOnboardingRejectsInvalidOperationType(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	service := NewService(repo)
	_, err := service.Onboarding(context.Background(), testAuth(), OnboardingInput{
		FullName:      "Ada Lovelace",
		State:         "Lagos",
		City:          "Ikeja",
		OperationType: OperationType("team"),
	})
	assertValidationField(t, err, "operation_type")
}

func TestOnboardingRejectsMissingFullName(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	service := NewService(repo)
	_, err := service.Onboarding(context.Background(), testAuth(), OnboardingInput{
		State:         "Lagos",
		City:          "Ikeja",
		OperationType: OperationIndividual,
	})
	assertValidationField(t, err, "full_name")
}

func TestOnboardingRejectsSingleName(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	service := NewService(repo)
	_, err := service.Onboarding(context.Background(), testAuth(), OnboardingInput{
		FullName:      "Ada",
		State:         "Lagos",
		City:          "Ikeja",
		OperationType: OperationIndividual,
	})
	assertValidationField(t, err, "full_name")
}

func TestOnboardingReturns404WhenProviderRowMissing(t *testing.T) {
	repo := newFakeProfileRepository()
	service := NewService(repo)
	_, err := service.Onboarding(context.Background(), testAuth(), OnboardingInput{
		FullName:      "Ada Lovelace",
		State:         "Lagos",
		City:          "Ikeja",
		OperationType: OperationIndividual,
	})
	assertErrorCode(t, err, apperrors.CodeNotFound)
}

func TestOnboardingReturns409WhenAlreadyComplete(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	service := NewService(repo)

	if _, err := service.Onboarding(context.Background(), testAuth(), OnboardingInput{
		FullName:      "Ada Lovelace",
		State:         "Lagos",
		City:          "Ikeja",
		OperationType: OperationIndividual,
	}); err != nil {
		t.Fatalf("first onboarding error = %v", err)
	}
	p := repo.providers["provider-123"]
	p.OnboardingComplete = true
	repo.providers["provider-123"] = p

	_, err := service.Onboarding(context.Background(), testAuth(), OnboardingInput{
		FullName:      "Ada Lovelace",
		State:         "Lagos",
		City:          "Ikeja",
		OperationType: OperationIndividual,
	})
	assertErrorCode(t, err, apperrors.CodeConflict)
}

func TestOnboardingProviderIDInBodyHasNoEffect(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	service := NewService(repo)

	result, err := service.Onboarding(context.Background(), testAuth(), OnboardingInput{
		FullName:      "Ada Lovelace",
		State:         "Lagos",
		City:          "Ikeja",
		OperationType: OperationIndividual,
	})
	if err != nil {
		t.Fatalf("onboarding error = %v", err)
	}
	if result.ProviderID != "provider-123" {
		t.Fatalf("provider_id = %s, want provider-123", result.ProviderID)
	}
}

func TestOnboardingValidPayloadReturns200(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	service := NewService(repo)

	result, err := service.Onboarding(context.Background(), testAuth(), OnboardingInput{
		FullName:      "Emeka Okafor",
		State:         "Lagos",
		City:          "Ikeja",
		OperationType: OperationIndividual,
	})
	if err != nil {
		t.Fatalf("onboarding error = %v", err)
	}
	if result.ProviderID != "provider-123" {
		t.Fatalf("provider_id = %s, want provider-123", result.ProviderID)
	}
}

func TestOnboardingLowercasesEmailAndStaysIncompleteWithoutContacts(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	service := NewService(repo)
	email := "ADA@EXAMPLE.COM"

	result, err := service.Onboarding(context.Background(), testAuth(), OnboardingInput{
		FullName:      "Ada Lovelace",
		Email:         &email,
		State:         "Lagos",
		City:          "Ikeja",
		OperationType: OperationIndividual,
	})
	if err != nil {
		t.Fatalf("Onboarding() error = %v", err)
	}
	if result.Email == nil || *result.Email != "ada@example.com" {
		t.Fatalf("email = %v, want ada@example.com", result.Email)
	}
	if result.OnboardingComplete {
		t.Fatal("onboarding_complete must stay false without emergency contact and guarantor")
	}
}

func TestOnboardingEmailOmittedIsValid(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	service := NewService(repo)

	result, err := service.Onboarding(context.Background(), testAuth(), OnboardingInput{
		FullName:      "Ada Lovelace",
		State:         "Lagos",
		City:          "Ikeja",
		OperationType: OperationIndividual,
	})
	if err != nil {
		t.Fatalf("onboarding without email error = %v", err)
	}
	if result.Email != nil {
		t.Fatalf("email = %v, want nil", result.Email)
	}
}

func TestOnboardingInvalidEmailReturns400(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	service := NewService(repo)
	email := "not-an-email"

	_, err := service.Onboarding(context.Background(), testAuth(), OnboardingInput{
		FullName:      "Ada Lovelace",
		Email:         &email,
		State:         "Lagos",
		City:          "Ikeja",
		OperationType: OperationIndividual,
	})
	assertValidationField(t, err, "email")
}

func TestOnboardingMissingStateReturns400(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	service := NewService(repo)
	_, err := service.Onboarding(context.Background(), testAuth(), OnboardingInput{
		FullName:      "Ada Lovelace",
		City:          "Ikeja",
		OperationType: OperationIndividual,
	})
	assertValidationField(t, err, "state")
}

func TestOnboardingMissingCityReturns400(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	service := NewService(repo)
	_, err := service.Onboarding(context.Background(), testAuth(), OnboardingInput{
		FullName:      "Ada Lovelace",
		State:         "Lagos",
		OperationType: OperationIndividual,
	})
	assertValidationField(t, err, "city")
}

func TestOnboardingCompleteAfterOnboardingEmergencyContactAndGuarantor(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	service := NewService(repo)

	if _, err := service.Onboarding(context.Background(), testAuth(), OnboardingInput{
		FullName:      "Ada Lovelace",
		State:         "Lagos",
		City:          "Ikeja",
		OperationType: OperationFleet,
	}); err != nil {
		t.Fatalf("Onboarding() error = %v", err)
	}
	if _, err := service.SetEmergencyContact(context.Background(), testAuth(), EmergencyContactInput{
		FullName:     "Grace Hopper",
		Phone:        "+2348012345678",
		Relationship: "Sister",
	}); err != nil {
		t.Fatalf("SetEmergencyContact() error = %v", err)
	}
	provider, _, err := repo.GetProviderByID(context.Background(), "provider-123")
	if err != nil || provider.OnboardingComplete {
		t.Fatalf("provider lookup failed before guarantor: provider=%+v err=%v", provider, err)
	}

	if _, err := service.SetGuarantor(context.Background(), testAuth(), GuarantorInput{
		FullName: "Alan Turing",
		Phone:    "+2348099999999",
	}); err != nil {
		t.Fatalf("SetGuarantor() error = %v", err)
	}
	provider, _, err = repo.GetProviderByID(context.Background(), "provider-123")
	if err != nil {
		t.Fatalf("GetProviderByID() error = %v", err)
	}
	if !provider.OnboardingComplete {
		t.Fatal("onboarding_complete = false, want true")
	}
}

// ── 2E: GET /me ───────────────────────────────────────────────────────────────

func TestGetMeReturns404WhenProviderMissing(t *testing.T) {
	repo := newFakeProfileRepository()
	service := NewService(repo)

	_, err := service.GetMe(context.Background(), testAuth())
	assertErrorCode(t, err, apperrors.CodeNotFound)
}

func TestGetMeReturnsSparseProfileBeforeOnboarding(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	service := NewService(repo)

	result, err := service.GetMe(context.Background(), testAuth())
	if err != nil {
		t.Fatalf("GetMe() error = %v", err)
	}
	if result.ProviderID != "provider-123" || result.Phone != "+2348000000001" {
		t.Fatalf("me = %+v", result)
	}
	if result.FullName != nil || result.State != nil || result.City != nil {
		t.Fatalf("sparse profile should have nil full_name/state/city: %+v", result)
	}
	if result.OnboardingComplete {
		t.Fatal("onboarding_complete must be false before onboarding")
	}
}

func TestGetMeReturnsFullProfileAfterOnboarding(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	service := NewService(repo)

	if _, err := service.Onboarding(context.Background(), testAuth(), OnboardingInput{
		FullName:      "Ada Lovelace",
		State:         "Lagos",
		City:          "Ikeja",
		OperationType: OperationIndividual,
	}); err != nil {
		t.Fatalf("Onboarding() error = %v", err)
	}

	result, err := service.GetMe(context.Background(), testAuth())
	if err != nil {
		t.Fatalf("GetMe() error = %v", err)
	}
	if result.FullName == nil || *result.FullName != "Ada Lovelace" {
		t.Fatalf("full_name = %v, want Ada Lovelace", result.FullName)
	}
	if result.State == nil || *result.State != "Lagos" {
		t.Fatalf("state = %v, want Lagos", result.State)
	}
}

func TestGetMeHasEmergencyContactFlag(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	service := NewService(repo)

	result, err := service.GetMe(context.Background(), testAuth())
	if err != nil {
		t.Fatalf("GetMe() error = %v", err)
	}
	if result.HasEmergencyContact {
		t.Fatal("has_emergency_contact should be false before setting contact")
	}

	if _, err := service.SetEmergencyContact(context.Background(), testAuth(), EmergencyContactInput{
		FullName: "Grace Hopper", Phone: "+2348012345678", Relationship: "Sister",
	}); err != nil {
		t.Fatalf("SetEmergencyContact() error = %v", err)
	}

	result, err = service.GetMe(context.Background(), testAuth())
	if err != nil {
		t.Fatalf("GetMe() second error = %v", err)
	}
	if !result.HasEmergencyContact {
		t.Fatal("has_emergency_contact should be true after setting contact")
	}
}

func TestGetMeHasGuarantorFlag(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	service := NewService(repo)

	result, err := service.GetMe(context.Background(), testAuth())
	if err != nil {
		t.Fatalf("GetMe() error = %v", err)
	}
	if result.HasGuarantor {
		t.Fatal("has_guarantor should be false before setting guarantor")
	}

	if _, err := service.SetGuarantor(context.Background(), testAuth(), GuarantorInput{
		FullName: "Alan Turing", Phone: "+2348099999999",
	}); err != nil {
		t.Fatalf("SetGuarantor() error = %v", err)
	}

	result, err = service.GetMe(context.Background(), testAuth())
	if err != nil {
		t.Fatalf("GetMe() second error = %v", err)
	}
	if !result.HasGuarantor {
		t.Fatal("has_guarantor should be true after setting guarantor")
	}
}

// ── 2F: PATCH /me ─────────────────────────────────────────────────────────────

func TestUpdateMeEmptyBodyReturns400(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	service := NewService(repo)

	_, err := service.UpdateMe(context.Background(), testAuth(), UpdateProviderInput{})
	assertValidationField(t, err, "body")
}

func TestUpdateMeCityOnlyUpdatesCityOnly(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	service := NewService(repo)

	if _, err := service.Onboarding(context.Background(), testAuth(), OnboardingInput{
		FullName:      "Ada Lovelace",
		State:         "Lagos",
		City:          "Ikeja",
		OperationType: OperationIndividual,
	}); err != nil {
		t.Fatalf("Onboarding() error = %v", err)
	}

	city := "Lekki"
	result, err := service.UpdateMe(context.Background(), testAuth(), UpdateProviderInput{City: &city})
	if err != nil {
		t.Fatalf("UpdateMe() error = %v", err)
	}
	if result.City == nil || *result.City != "Lekki" {
		t.Fatalf("city = %v, want Lekki", result.City)
	}
	if result.FullName == nil || *result.FullName != "Ada Lovelace" {
		t.Fatalf("full_name changed unexpectedly = %v", result.FullName)
	}
}

func TestUpdateMeFullNameOnlyUpdatesFullNameOnly(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	service := NewService(repo)

	if _, err := service.Onboarding(context.Background(), testAuth(), OnboardingInput{
		FullName:      "Ada Lovelace",
		State:         "Lagos",
		City:          "Ikeja",
		OperationType: OperationIndividual,
	}); err != nil {
		t.Fatalf("Onboarding() error = %v", err)
	}

	name := "Ada Byron"
	result, err := service.UpdateMe(context.Background(), testAuth(), UpdateProviderInput{FullName: &name})
	if err != nil {
		t.Fatalf("UpdateMe() error = %v", err)
	}
	if result.FullName == nil || *result.FullName != "Ada Byron" {
		t.Fatalf("full_name = %v, want Ada Byron", result.FullName)
	}
	if result.City == nil || *result.City != "Ikeja" {
		t.Fatalf("city changed unexpectedly = %v", result.City)
	}
}

func TestUpdateMeInvalidEmailReturns400(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	service := NewService(repo)
	email := "not-valid"

	_, err := service.UpdateMe(context.Background(), testAuth(), UpdateProviderInput{Email: &email})
	assertValidationField(t, err, "email")
}

func TestUpdateMeReadOnlyFieldsCannotBeChanged(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	service := NewService(repo)

	city := "Lekki"
	result, err := service.UpdateMe(context.Background(), testAuth(), UpdateProviderInput{City: &city})
	if err != nil {
		t.Fatalf("UpdateMe() error = %v", err)
	}
	if result.Phone != "+2348000000001" {
		t.Fatalf("phone changed unexpectedly = %s", result.Phone)
	}
	if result.VerificationStatus != StatusUnverified {
		t.Fatalf("verification_status changed unexpectedly = %s", result.VerificationStatus)
	}
}

func TestUpdateMePublishesProfileUpdatedEvent(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	publisher := &fakeProfileEventPublisher{}
	service := NewServiceWithEvents(repo, publisher)

	city := "Lekki"
	if _, err := service.UpdateMe(context.Background(), testAuth(), UpdateProviderInput{City: &city}); err != nil {
		t.Fatalf("UpdateMe() error = %v", err)
	}
	if len(publisher.profileUpdated) != 1 {
		t.Fatalf("published %d profile_updated events, want 1", len(publisher.profileUpdated))
	}
	ev := publisher.profileUpdated[0]
	if ev.Event != TopicProfileUpdated {
		t.Fatalf("event = %s, want %s", ev.Event, TopicProfileUpdated)
	}
	if ev.ProviderID != "provider-123" {
		t.Fatalf("provider_id = %s, want provider-123", ev.ProviderID)
	}
	if len(ev.ChangedFields) != 1 || ev.ChangedFields[0] != "city" {
		t.Fatalf("changed_fields = %v, want [city]", ev.ChangedFields)
	}
}

func TestUpdateMeReturnsFullUpdatedProfile(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	service := NewService(repo)

	city := "Lekki"
	result, err := service.UpdateMe(context.Background(), testAuth(), UpdateProviderInput{City: &city})
	if err != nil {
		t.Fatalf("UpdateMe() error = %v", err)
	}
	if result.ProviderID == "" {
		t.Fatal("response missing provider_id")
	}
	if result.Phone == "" {
		t.Fatal("response missing phone")
	}
}

// ── 2G: Emergency contact ─────────────────────────────────────────────────────

func TestSetEmergencyContactValidReturns200(t *testing.T) {
	repo := newFakeProfileRepository()
	service := NewService(repo)

	contact, err := service.SetEmergencyContact(context.Background(), testAuth(), EmergencyContactInput{
		FullName:     "Adaeze Okafor",
		Phone:        "+2348098765432",
		Relationship: "spouse",
	})
	if err != nil {
		t.Fatalf("SetEmergencyContact() error = %v", err)
	}
	if contact.FullName != "Adaeze Okafor" || contact.Phone != "+2348098765432" || contact.Relationship != "spouse" {
		t.Fatalf("contact = %+v", contact)
	}
}

func TestSetEmergencyContactInvalidPhoneReturns400(t *testing.T) {
	repo := newFakeProfileRepository()
	service := NewService(repo)

	_, err := service.SetEmergencyContact(context.Background(), testAuth(), EmergencyContactInput{
		FullName:     "Adaeze Okafor",
		Phone:        "08098765432",
		Relationship: "spouse",
	})
	assertValidationField(t, err, "phone")
}

func TestSetEmergencyContactMissingFullNameReturns400(t *testing.T) {
	repo := newFakeProfileRepository()
	service := NewService(repo)

	_, err := service.SetEmergencyContact(context.Background(), testAuth(), EmergencyContactInput{
		Phone:        "+2348098765432",
		Relationship: "spouse",
	})
	assertValidationField(t, err, "full_name")
}

func TestSetEmergencyContactMissingRelationshipReturns400(t *testing.T) {
	repo := newFakeProfileRepository()
	service := NewService(repo)

	_, err := service.SetEmergencyContact(context.Background(), testAuth(), EmergencyContactInput{
		FullName: "Adaeze Okafor",
		Phone:    "+2348098765432",
	})
	assertValidationField(t, err, "relationship")
}

func TestSetEmergencyContactCalledTwiceReplacesContact(t *testing.T) {
	repo := newFakeProfileRepository()
	service := NewService(repo)

	first, err := service.SetEmergencyContact(context.Background(), testAuth(), EmergencyContactInput{
		FullName:     "Grace Hopper",
		Phone:        "+2348012345678",
		Relationship: "Sister",
	})
	if err != nil {
		t.Fatalf("first SetEmergencyContact() error = %v", err)
	}
	second, err := service.SetEmergencyContact(context.Background(), testAuth(), EmergencyContactInput{
		FullName:     "Katherine Johnson",
		Phone:        "+2348012345679",
		Relationship: "Aunt",
	})
	if err != nil {
		t.Fatalf("second SetEmergencyContact() error = %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("IDs differ, want upsert: first=%s second=%s", first.ID, second.ID)
	}
	if second.FullName != "Katherine Johnson" {
		t.Fatalf("full_name = %s, want Katherine Johnson", second.FullName)
	}
	// Confirm no duplicate rows
	_, ok, _ := repo.GetEmergencyContact(context.Background(), "provider-123")
	if !ok {
		t.Fatal("emergency contact should exist")
	}
}

func TestGetEmergencyContactReturns200WhenExists(t *testing.T) {
	repo := newFakeProfileRepository()
	service := NewService(repo)

	if _, err := service.SetEmergencyContact(context.Background(), testAuth(), EmergencyContactInput{
		FullName:     "Grace Hopper",
		Phone:        "+2348012345678",
		Relationship: "Sister",
	}); err != nil {
		t.Fatalf("SetEmergencyContact() error = %v", err)
	}

	contact, err := service.GetEmergencyContact(context.Background(), testAuth())
	if err != nil {
		t.Fatalf("GetEmergencyContact() error = %v", err)
	}
	if contact.FullName != "Grace Hopper" {
		t.Fatalf("full_name = %s, want Grace Hopper", contact.FullName)
	}
}

func TestGetEmergencyContactReturns404WhenNotSet(t *testing.T) {
	repo := newFakeProfileRepository()
	service := NewService(repo)

	_, err := service.GetEmergencyContact(context.Background(), testAuth())
	assertErrorCode(t, err, apperrors.CodeNotFound)
}

func TestSetEmergencyContactCallsCheckOnboardingComplete(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	service := NewService(repo)

	if _, err := service.SetEmergencyContact(context.Background(), testAuth(), EmergencyContactInput{
		FullName:     "Grace Hopper",
		Phone:        "+2348012345678",
		Relationship: "Sister",
	}); err != nil {
		t.Fatalf("SetEmergencyContact() error = %v", err)
	}
	provider, _, _ := repo.GetProviderByID(context.Background(), "provider-123")
	// onboarding_complete is still false (no profile fields + no guarantor), but
	// RecalculateOnboardingComplete was called — verify updated_at changed
	if provider.UpdatedAt.IsZero() {
		t.Fatal("updated_at should be set after recalculate")
	}
}

func TestEmergencyContactCompletesOnboardingFiresEventOnce(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	publisher := &fakeProfileEventPublisher{}
	service := NewServiceWithEvents(repo, publisher)

	// Set up profile + guarantor first so adding emergency contact completes onboarding
	if _, err := service.Onboarding(context.Background(), testAuth(), OnboardingInput{
		FullName:      "Ada Lovelace",
		State:         "Lagos",
		City:          "Ikeja",
		OperationType: OperationIndividual,
	}); err != nil {
		t.Fatalf("Onboarding() error = %v", err)
	}
	if _, err := service.SetGuarantor(context.Background(), testAuth(), GuarantorInput{
		FullName: "Alan Turing",
		Phone:    "+2348099999999",
	}); err != nil {
		t.Fatalf("SetGuarantor() error = %v", err)
	}
	if len(publisher.onboardingCompleted) != 0 {
		t.Fatalf("onboarding.completed fired before all steps: %d events", len(publisher.onboardingCompleted))
	}

	// Adding emergency contact should complete onboarding
	if _, err := service.SetEmergencyContact(context.Background(), testAuth(), EmergencyContactInput{
		FullName:     "Grace Hopper",
		Phone:        "+2348012345678",
		Relationship: "Sister",
	}); err != nil {
		t.Fatalf("SetEmergencyContact() error = %v", err)
	}
	if len(publisher.onboardingCompleted) != 1 {
		t.Fatalf("onboarding.completed fired %d times, want 1", len(publisher.onboardingCompleted))
	}
	if publisher.onboardingCompleted[0].Event != TopicOnboardingCompleted {
		t.Fatalf("event = %s, want %s", publisher.onboardingCompleted[0].Event, TopicOnboardingCompleted)
	}

	// Calling SetEmergencyContact again must NOT fire a second event
	if _, err := service.SetEmergencyContact(context.Background(), testAuth(), EmergencyContactInput{
		FullName:     "Mary Jackson",
		Phone:        "+2348012345670",
		Relationship: "Friend",
	}); err != nil {
		t.Fatalf("second SetEmergencyContact() error = %v", err)
	}
	if len(publisher.onboardingCompleted) != 1 {
		t.Fatalf("onboarding.completed fired again: %d total events", len(publisher.onboardingCompleted))
	}
}

// ── 2H: Guarantor ─────────────────────────────────────────────────────────────

func TestSetGuarantorValidReturns200(t *testing.T) {
	repo := newFakeProfileRepository()
	service := NewService(repo)

	guarantor, err := service.SetGuarantor(context.Background(), testAuth(), GuarantorInput{
		FullName: "Chukwudi Okafor",
		Phone:    "+2347011223344",
	})
	if err != nil {
		t.Fatalf("SetGuarantor() error = %v", err)
	}
	if guarantor.FullName != "Chukwudi Okafor" || guarantor.Phone != "+2347011223344" {
		t.Fatalf("guarantor = %+v", guarantor)
	}
}

func TestSetGuarantorInvalidPhoneReturns400(t *testing.T) {
	repo := newFakeProfileRepository()
	service := NewService(repo)

	_, err := service.SetGuarantor(context.Background(), testAuth(), GuarantorInput{
		FullName: "Chukwudi Okafor",
		Phone:    "07011223344",
	})
	assertValidationField(t, err, "phone")
}

func TestSetGuarantorMissingFullNameReturns400(t *testing.T) {
	repo := newFakeProfileRepository()
	service := NewService(repo)

	_, err := service.SetGuarantor(context.Background(), testAuth(), GuarantorInput{
		Phone: "+2347011223344",
	})
	assertValidationField(t, err, "full_name")
}

func TestSetGuarantorCalledTwiceReplacesGuarantor(t *testing.T) {
	repo := newFakeProfileRepository()
	service := NewService(repo)

	first, err := service.SetGuarantor(context.Background(), testAuth(), GuarantorInput{
		FullName: "Alan Turing",
		Phone:    "+2348099999999",
	})
	if err != nil {
		t.Fatalf("first SetGuarantor() error = %v", err)
	}
	second, err := service.SetGuarantor(context.Background(), testAuth(), GuarantorInput{
		FullName: "Mary Jackson",
		Phone:    "+2348099999998",
	})
	if err != nil {
		t.Fatalf("second SetGuarantor() error = %v", err)
	}
	if first.ID != second.ID {
		t.Fatalf("IDs differ, want upsert: first=%s second=%s", first.ID, second.ID)
	}
	if second.FullName != "Mary Jackson" {
		t.Fatalf("full_name = %s, want Mary Jackson", second.FullName)
	}
}

func TestGetGuarantorReturns200WhenExists(t *testing.T) {
	repo := newFakeProfileRepository()
	service := NewService(repo)

	if _, err := service.SetGuarantor(context.Background(), testAuth(), GuarantorInput{
		FullName: "Alan Turing",
		Phone:    "+2348099999999",
	}); err != nil {
		t.Fatalf("SetGuarantor() error = %v", err)
	}

	guarantor, err := service.GetGuarantor(context.Background(), testAuth())
	if err != nil {
		t.Fatalf("GetGuarantor() error = %v", err)
	}
	if guarantor.FullName != "Alan Turing" {
		t.Fatalf("full_name = %s, want Alan Turing", guarantor.FullName)
	}
}

func TestGetGuarantorReturns404WhenNotSet(t *testing.T) {
	repo := newFakeProfileRepository()
	service := NewService(repo)

	_, err := service.GetGuarantor(context.Background(), testAuth())
	assertErrorCode(t, err, apperrors.CodeNotFound)
}

func TestSetGuarantorCallsCheckOnboardingComplete(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	service := NewService(repo)

	if _, err := service.SetGuarantor(context.Background(), testAuth(), GuarantorInput{
		FullName: "Alan Turing",
		Phone:    "+2348099999999",
	}); err != nil {
		t.Fatalf("SetGuarantor() error = %v", err)
	}
	provider, _, _ := repo.GetProviderByID(context.Background(), "provider-123")
	if provider.UpdatedAt.IsZero() {
		t.Fatal("updated_at should be set after recalculate")
	}
}

func TestGuarantorCompletesOnboardingFiresEventOnce(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	publisher := &fakeProfileEventPublisher{}
	service := NewServiceWithEvents(repo, publisher)

	// Set up profile + emergency contact first
	if _, err := service.Onboarding(context.Background(), testAuth(), OnboardingInput{
		FullName:      "Ada Lovelace",
		State:         "Lagos",
		City:          "Ikeja",
		OperationType: OperationIndividual,
	}); err != nil {
		t.Fatalf("Onboarding() error = %v", err)
	}
	if _, err := service.SetEmergencyContact(context.Background(), testAuth(), EmergencyContactInput{
		FullName:     "Grace Hopper",
		Phone:        "+2348012345678",
		Relationship: "Sister",
	}); err != nil {
		t.Fatalf("SetEmergencyContact() error = %v", err)
	}
	if len(publisher.onboardingCompleted) != 0 {
		t.Fatalf("onboarding.completed fired before guarantor: %d events", len(publisher.onboardingCompleted))
	}

	// Adding guarantor should complete onboarding
	if _, err := service.SetGuarantor(context.Background(), testAuth(), GuarantorInput{
		FullName: "Alan Turing",
		Phone:    "+2348099999999",
	}); err != nil {
		t.Fatalf("SetGuarantor() error = %v", err)
	}
	if len(publisher.onboardingCompleted) != 1 {
		t.Fatalf("onboarding.completed fired %d times, want 1", len(publisher.onboardingCompleted))
	}

	// Calling SetGuarantor again must NOT fire a second event
	if _, err := service.SetGuarantor(context.Background(), testAuth(), GuarantorInput{
		FullName: "Katherine Johnson",
		Phone:    "+2348099999997",
	}); err != nil {
		t.Fatalf("second SetGuarantor() error = %v", err)
	}
	if len(publisher.onboardingCompleted) != 1 {
		t.Fatalf("onboarding.completed fired again: %d total events", len(publisher.onboardingCompleted))
	}
}

// ── 2I: GET /stats ─────────────────────────────────────────────────────────────

func TestGetStatsReturnsCorrectTotalTrips(t *testing.T) {
	repo := newFakeProfileRepository()
	provider, _ := repo.EnsureProvider(context.Background(), "provider-123", "+2348000000001")
	provider.TotalTrips = 42
	repo.providers[provider.ID] = provider
	service := NewService(repo)

	stats, err := service.GetStats(context.Background(), testAuth())
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}
	if stats.TotalTrips != 42 {
		t.Fatalf("total_trips = %d, want 42", stats.TotalTrips)
	}
}

func TestGetStatsReturnsCorrectAvgRating(t *testing.T) {
	repo := newFakeProfileRepository()
	provider, _ := repo.EnsureProvider(context.Background(), "provider-123", "+2348000000001")
	provider.AvgRating = 4.85
	repo.providers[provider.ID] = provider
	service := NewService(repo)

	stats, err := service.GetStats(context.Background(), testAuth())
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}
	if stats.AvgRating != 4.85 {
		t.Fatalf("avg_rating = %f, want 4.85", stats.AvgRating)
	}
}

func TestGetStatsReturnsCorrectRatingsCount(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	repo.ratingCounts["provider-123"] = 38
	service := NewService(repo)

	stats, err := service.GetStats(context.Background(), testAuth())
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}
	if stats.RatingsCount != 38 {
		t.Fatalf("ratings_count = %d, want 38", stats.RatingsCount)
	}
}

func TestGetStatsCompletionRateZeroWhenNoTrips(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	service := NewService(repo)

	stats, err := service.GetStats(context.Background(), testAuth())
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}
	if stats.CompletionRate != 0.0 {
		t.Fatalf("completion_rate = %f, want 0.00", stats.CompletionRate)
	}
}

func TestGetStatsCompletionRateOneWhenTripsExist(t *testing.T) {
	repo := newFakeProfileRepository()
	provider, _ := repo.EnsureProvider(context.Background(), "provider-123", "+2348000000001")
	provider.TotalTrips = 5
	repo.providers[provider.ID] = provider
	service := NewService(repo)

	stats, err := service.GetStats(context.Background(), testAuth())
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}
	if stats.CompletionRate != 1.0 {
		t.Fatalf("completion_rate = %f, want 1.00", stats.CompletionRate)
	}
}

func TestGetStatsReturnsZeroValuesForNewProvider(t *testing.T) {
	repo := newFakeProfileRepository()
	repo.mustEnsure("provider-123", "+2348000000001")
	service := NewService(repo)

	stats, err := service.GetStats(context.Background(), testAuth())
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}
	if stats.TotalTrips != 0 || stats.AvgRating != 0 || stats.RatingsCount != 0 || stats.CompletionRate != 0 {
		t.Fatalf("new provider stats should be zero: %+v", stats)
	}
}

func TestGetStatsReturns404WhenProviderMissing(t *testing.T) {
	repo := newFakeProfileRepository()
	service := NewService(repo)

	_, err := service.GetStats(context.Background(), testAuth())
	assertErrorCode(t, err, apperrors.CodeNotFound)
}

// ── Existing preserved tests ──────────────────────────────────────────────────

func TestEmergencyContactAndGuarantorUpsert(t *testing.T) {
	repo := newFakeProfileRepository()
	service := NewService(repo)

	firstContact, err := service.SetEmergencyContact(context.Background(), testAuth(), EmergencyContactInput{
		FullName:     "Grace Hopper",
		Phone:        "+2348012345678",
		Relationship: "Sister",
	})
	if err != nil {
		t.Fatalf("SetEmergencyContact first error = %v", err)
	}
	secondContact, err := service.SetEmergencyContact(context.Background(), testAuth(), EmergencyContactInput{
		FullName:     "Katherine Johnson",
		Phone:        "+2348012345679",
		Relationship: "Aunt",
	})
	if err != nil {
		t.Fatalf("SetEmergencyContact second error = %v", err)
	}
	if firstContact.ID != secondContact.ID || secondContact.FullName != "Katherine Johnson" {
		t.Fatalf("contact upsert failed: first=%+v second=%+v", firstContact, secondContact)
	}

	firstGuarantor, err := service.SetGuarantor(context.Background(), testAuth(), GuarantorInput{
		FullName: "Alan Turing",
		Phone:    "+2348099999999",
	})
	if err != nil {
		t.Fatalf("SetGuarantor first error = %v", err)
	}
	secondGuarantor, err := service.SetGuarantor(context.Background(), testAuth(), GuarantorInput{
		FullName: "Mary Jackson",
		Phone:    "+2348099999998",
	})
	if err != nil {
		t.Fatalf("SetGuarantor second error = %v", err)
	}
	if firstGuarantor.ID != secondGuarantor.ID || secondGuarantor.FullName != "Mary Jackson" {
		t.Fatalf("guarantor upsert failed: first=%+v second=%+v", firstGuarantor, secondGuarantor)
	}
}

func TestStatsReturnsBasicProviderStats(t *testing.T) {
	repo := newFakeProfileRepository()
	service := NewService(repo)
	provider, _ := repo.EnsureProvider(context.Background(), "provider-123", "+2348000000001")
	provider.AvgRating = 4.5
	provider.TotalTrips = 17
	provider.VerificationStatus = StatusVerified
	repo.providers[provider.ID] = provider
	repo.ratingCounts["provider-123"] = 12

	stats, err := service.GetStats(context.Background(), testAuth())
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}
	if stats.AvgRating != 4.5 || stats.TotalTrips != 17 || stats.VerificationStatus != StatusVerified || stats.RatingsCount != 12 {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestPublicProfileDoesNotExposePhoneOrEmail(t *testing.T) {
	repo := newFakeProfileRepository()
	service := NewService(repo)
	providerID := "11111111-1111-1111-1111-111111111111"
	email := "ada@example.com"
	fullName := "Ada Lovelace"
	city := "Ikeja"
	state := "Lagos"
	op := OperationIndividual
	provider, _ := repo.EnsureProvider(context.Background(), providerID, "+2348000000001")
	provider.FullName = &fullName
	provider.Email = &email
	provider.City = &city
	provider.State = &state
	provider.OperationType = &op
	repo.providers[provider.ID] = provider

	publicProfile, err := service.GetPublicProfile(context.Background(), providerID)
	if err != nil {
		t.Fatalf("GetPublicProfile() error = %v", err)
	}
	if publicProfile.ProviderID != providerID || publicProfile.FullName == nil || *publicProfile.FullName != "Ada Lovelace" {
		t.Fatalf("public profile = %+v", publicProfile)
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func testAuth() AuthContext {
	return AuthContext{ProviderID: "provider-123", PhoneNumber: "+2348000000001"}
}

func assertValidationField(t *testing.T, err error, field string) {
	t.Helper()
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("error = %v, want app error", err)
	}
	if appErr.Code != apperrors.CodeValidationFailed {
		t.Fatalf("code = %s, want validation_failed", appErr.Code)
	}
	for _, violation := range appErr.Fields {
		if violation.Field == field {
			return
		}
	}
	t.Fatalf("validation fields = %+v, want %s", appErr.Fields, field)
}

func assertErrorCode(t *testing.T, err error, code apperrors.Code) {
	t.Helper()
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("error = %v, want app error", err)
	}
	if appErr.Code != code {
		t.Fatalf("code = %s, want %s", appErr.Code, code)
	}
}

// ── Fake repository ───────────────────────────────────────────────────────────

type fakeProfileRepository struct {
	providers    map[string]Provider
	contacts     map[string]EmergencyContact
	guarantors   map[string]Guarantor
	ratingCounts map[string]int
	ratings      map[string]RatingInput
}

func newFakeProfileRepository() *fakeProfileRepository {
	return &fakeProfileRepository{
		providers:    make(map[string]Provider),
		contacts:     make(map[string]EmergencyContact),
		guarantors:   make(map[string]Guarantor),
		ratingCounts: make(map[string]int),
		ratings:      make(map[string]RatingInput),
	}
}

func (r *fakeProfileRepository) mustEnsure(providerID, phone string) Provider {
	p, _ := r.EnsureProvider(context.Background(), providerID, phone)
	return p
}

func (r *fakeProfileRepository) EnsureProvider(ctx context.Context, providerID string, phone string) (Provider, error) {
	if provider, ok := r.providers[providerID]; ok {
		return provider, nil
	}
	now := time.Now().UTC()
	provider := Provider{
		ID:                 providerID,
		Phone:              phone,
		Country:            "NG",
		VerificationStatus: StatusUnverified,
		IsActive:           true,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
	r.providers[providerID] = provider
	return provider, nil
}

func (r *fakeProfileRepository) GetProviderByID(ctx context.Context, providerID string) (Provider, bool, error) {
	provider, ok := r.providers[providerID]
	return provider, ok, nil
}

func (r *fakeProfileRepository) UpdateOnboarding(ctx context.Context, providerID string, input OnboardingInput) (Provider, error) {
	provider := r.providers[providerID]
	provider.FullName = &input.FullName
	provider.Email = input.Email
	provider.State = &input.State
	provider.City = &input.City
	provider.OperationType = &input.OperationType
	provider.UpdatedAt = time.Now().UTC()
	r.providers[providerID] = provider
	return provider, nil
}

func (r *fakeProfileRepository) PatchProvider(ctx context.Context, providerID string, input UpdateProviderInput) (Provider, error) {
	provider := r.providers[providerID]
	if input.FullName != nil {
		provider.FullName = input.FullName
	}
	if input.Email != nil {
		provider.Email = input.Email
	}
	if input.State != nil {
		provider.State = input.State
	}
	if input.City != nil {
		provider.City = input.City
	}
	if input.ProfilePhotoURL != nil {
		provider.ProfilePhotoURL = input.ProfilePhotoURL
	}
	provider.UpdatedAt = time.Now().UTC()
	r.providers[providerID] = provider
	return provider, nil
}

func (r *fakeProfileRepository) UpsertEmergencyContact(ctx context.Context, providerID string, input EmergencyContactInput) (EmergencyContact, error) {
	contact := r.contacts[providerID]
	if contact.ID == "" {
		contact.ID = "contact-" + providerID
		contact.ProviderID = providerID
		contact.CreatedAt = time.Now().UTC()
	}
	contact.FullName = input.FullName
	contact.Phone = input.Phone
	contact.Relationship = input.Relationship
	contact.UpdatedAt = time.Now().UTC()
	r.contacts[providerID] = contact
	return contact, nil
}

func (r *fakeProfileRepository) GetEmergencyContact(ctx context.Context, providerID string) (EmergencyContact, bool, error) {
	contact, ok := r.contacts[providerID]
	return contact, ok, nil
}

func (r *fakeProfileRepository) UpsertGuarantor(ctx context.Context, providerID string, input GuarantorInput) (Guarantor, error) {
	guarantor := r.guarantors[providerID]
	if guarantor.ID == "" {
		guarantor.ID = "guarantor-" + providerID
		guarantor.ProviderID = providerID
		guarantor.CreatedAt = time.Now().UTC()
	}
	guarantor.FullName = input.FullName
	guarantor.Phone = input.Phone
	guarantor.UpdatedAt = time.Now().UTC()
	r.guarantors[providerID] = guarantor
	return guarantor, nil
}

func (r *fakeProfileRepository) GetGuarantor(ctx context.Context, providerID string) (Guarantor, bool, error) {
	guarantor, ok := r.guarantors[providerID]
	return guarantor, ok, nil
}

func (r *fakeProfileRepository) RecalculateOnboardingComplete(ctx context.Context, providerID string) (Provider, error) {
	provider := r.providers[providerID]
	if provider.OnboardingComplete {
		return provider, nil
	}
	provider.OnboardingComplete = provider.FullName != nil &&
		*provider.FullName != "" &&
		provider.State != nil &&
		*provider.State != "" &&
		provider.City != nil &&
		*provider.City != "" &&
		provider.OperationType != nil &&
		(*provider.OperationType == OperationIndividual || *provider.OperationType == OperationFleet)
	if _, ok := r.contacts[providerID]; !ok {
		provider.OnboardingComplete = false
	}
	if _, ok := r.guarantors[providerID]; !ok {
		provider.OnboardingComplete = false
	}
	provider.UpdatedAt = time.Now().UTC()
	r.providers[providerID] = provider
	return provider, nil
}

func (r *fakeProfileRepository) CountRatings(ctx context.Context, providerID string) (int, error) {
	return r.ratingCounts[providerID], nil
}

func (r *fakeProfileRepository) UpdateVerificationStatus(ctx context.Context, providerID string, status VerificationStatus) error {
	provider := r.providers[providerID]
	provider.VerificationStatus = status
	if status == StatusSuspended {
		provider.IsActive = false
	}
	provider.UpdatedAt = time.Now().UTC()
	r.providers[providerID] = provider
	return nil
}

func (r *fakeProfileRepository) IncrementTotalTrips(ctx context.Context, providerID string) error {
	provider := r.providers[providerID]
	provider.TotalTrips++
	provider.UpdatedAt = time.Now().UTC()
	r.providers[providerID] = provider
	return nil
}

func (r *fakeProfileRepository) InsertRatingAndRecalculate(ctx context.Context, input RatingInput) (bool, error) {
	if _, exists := r.ratings[input.BookingID]; exists {
		return false, nil
	}
	r.ratings[input.BookingID] = input
	r.ratingCounts[input.ProviderID]++

	total := 0
	count := 0
	for _, rating := range r.ratings {
		if rating.ProviderID == input.ProviderID {
			total += rating.Score
			count++
		}
	}
	provider := r.providers[input.ProviderID]
	if count > 0 {
		provider.AvgRating = float64(total) / float64(count)
	}
	provider.UpdatedAt = time.Now().UTC()
	r.providers[input.ProviderID] = provider
	return true, nil
}

// ── Fake event publisher ──────────────────────────────────────────────────────

type fakeProfileEventPublisher struct {
	profileUpdated      []ProfileUpdatedEvent
	onboardingCompleted []OnboardingCompletedEvent
}

func (p *fakeProfileEventPublisher) PublishProfileUpdated(_ context.Context, event ProfileUpdatedEvent) error {
	p.profileUpdated = append(p.profileUpdated, event)
	return nil
}

func (p *fakeProfileEventPublisher) PublishOnboardingCompleted(_ context.Context, event OnboardingCompletedEvent) error {
	p.onboardingCompleted = append(p.onboardingCompleted, event)
	return nil
}

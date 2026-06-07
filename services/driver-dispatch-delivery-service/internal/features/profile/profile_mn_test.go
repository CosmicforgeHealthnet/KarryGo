package profile

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"karrygo/shared/go/apperrors"
)

func TestGetMeReturnsOnlyAuthenticatedProviderProfile(t *testing.T) {
	repo := newFakeProfileRepository()
	ownName := "Ada Lovelace"
	otherName := "Grace Hopper"
	own, _ := repo.EnsureProvider(context.Background(), "provider-123", "+2348000000001")
	own.FullName = &ownName
	repo.providers[own.ID] = own
	other, _ := repo.EnsureProvider(context.Background(), "provider-456", "+2348000000002")
	other.FullName = &otherName
	repo.providers[other.ID] = other

	service := NewService(repo)
	result, err := service.GetMe(context.Background(), testAuth())
	if err != nil {
		t.Fatalf("GetMe() error = %v", err)
	}
	if result.ProviderID != "provider-123" {
		t.Fatalf("provider_id = %s, want authenticated provider", result.ProviderID)
	}
	if result.FullName == nil || *result.FullName != ownName {
		t.Fatalf("full_name = %v, want %s", result.FullName, ownName)
	}
}

func TestHandlerPatchMeRejectsAllReadOnlySecurityFields(t *testing.T) {
	cases := []struct {
		name  string
		field string
		body  string
	}{
		{name: "phone", field: "phone", body: `{"phone":"+2348012345678"}`},
		{name: "provider_id", field: "provider_id", body: `{"provider_id":"provider-456"}`},
		{name: "id", field: "id", body: `{"id":"provider-456"}`},
		{name: "verification_status", field: "verification_status", body: `{"verification_status":"verified"}`},
		{name: "avg_rating", field: "avg_rating", body: `{"avg_rating":5}`},
		{name: "total_trips", field: "total_trips", body: `{"total_trips":1000}`},
		{name: "is_active", field: "is_active", body: `{"is_active":false}`},
		{name: "onboarding_complete", field: "onboarding_complete", body: `{"onboarding_complete":true}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := newFakeProfileRepository()
			provider := repo.mustEnsure("provider-123", "+2348000000001")
			provider.VerificationStatus = StatusUnverified
			provider.IsActive = true
			repo.providers[provider.ID] = provider
			router, tokens := buildProfileTestRouter(repo)
			token := mustAccessToken(t, tokens)

			req := httptest.NewRequest(http.MethodPatch, "/api/v1/provider/me", bytes.NewBufferString(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
			}
			assertErrorCodeInBody(t, w.Body.Bytes(), apperrors.CodeValidationFailed)
			assertValidationFieldInBody(t, w.Body.Bytes(), tc.field)

			after := repo.providers["provider-123"]
			if after.Phone != "+2348000000001" || after.VerificationStatus != StatusUnverified ||
				after.AvgRating != 0 || after.TotalTrips != 0 || !after.IsActive || after.OnboardingComplete {
				t.Fatalf("read-only fields changed: %+v", after)
			}
		})
	}
}

func TestPatchMeRejectsInvalidProfilePhotoURL(t *testing.T) {
	cases := []string{
		`{"profile_photo_url":"http://cdn.example.com/avatar.png"}`,
		`{"profile_photo_url":"not-a-url"}`,
	}
	for _, body := range cases {
		t.Run(body, func(t *testing.T) {
			router, tokens := buildProfileTestRouter(newFakeProfileRepository())
			token := mustAccessToken(t, tokens)

			req := httptest.NewRequest(http.MethodPatch, "/api/v1/provider/me", bytes.NewBufferString(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
			}
			assertErrorCodeInBody(t, w.Body.Bytes(), apperrors.CodeValidationFailed)
			assertValidationFieldInBody(t, w.Body.Bytes(), "profile_photo_url")
		})
	}
}

func TestValidationRunsBeforeProfileDatabaseWrites(t *testing.T) {
	repo := &trackingProfileRepository{fakeProfileRepository: newFakeProfileRepository()}
	repo.mustEnsure("provider-123", "+2348000000001")
	service := NewService(repo)

	if _, err := service.Onboarding(context.Background(), testAuth(), OnboardingInput{
		State: "Lagos", City: "Ikeja", OperationType: OperationIndividual,
	}); err == nil {
		t.Fatal("expected onboarding validation error")
	}
	if repo.getProviderCalls != 0 || repo.updateOnboardingCalls != 0 {
		t.Fatalf("onboarding touched repository before validation: get=%d update=%d", repo.getProviderCalls, repo.updateOnboardingCalls)
	}

	email := "not-valid"
	if _, err := service.UpdateMe(context.Background(), testAuth(), UpdateProviderInput{Email: &email}); err == nil {
		t.Fatal("expected update validation error")
	}
	if repo.getProviderCalls != 0 || repo.patchProviderCalls != 0 {
		t.Fatalf("patch touched repository before validation: get=%d patch=%d", repo.getProviderCalls, repo.patchProviderCalls)
	}

	if _, err := service.SetEmergencyContact(context.Background(), testAuth(), EmergencyContactInput{
		FullName: "Adaeze Okafor", Phone: "08012345678", Relationship: "spouse",
	}); err == nil {
		t.Fatal("expected contact validation error")
	}
	if repo.ensureProviderCalls != 0 || repo.upsertContactCalls != 0 {
		t.Fatalf("contact touched repository before validation: ensure=%d upsert=%d", repo.ensureProviderCalls, repo.upsertContactCalls)
	}

	if _, err := service.SetGuarantor(context.Background(), testAuth(), GuarantorInput{
		FullName: "Alan Turing", Phone: "08012345678",
	}); err == nil {
		t.Fatal("expected guarantor validation error")
	}
	if repo.ensureProviderCalls != 0 || repo.upsertGuarantorCalls != 0 {
		t.Fatalf("guarantor touched repository before validation: ensure=%d upsert=%d", repo.ensureProviderCalls, repo.upsertGuarantorCalls)
	}

	if _, err := service.GetPublicProfile(context.Background(), "not-a-uuid"); err == nil {
		t.Fatal("expected public UUID validation error")
	}
	if repo.getProviderCalls != 0 {
		t.Fatalf("public UUID validation touched repository: get=%d", repo.getProviderCalls)
	}
}

func TestPublicProfileAllowsOnlyPublicFields(t *testing.T) {
	repo := newFakeProfileRepository()
	providerID := "55555555-5555-5555-5555-555555555555"
	fullName := "Ada Lovelace"
	email := "ada@example.com"
	state := "Lagos"
	city := "Ikeja"
	photo := "https://cdn.example.com/avatar.png"
	op := OperationIndividual
	provider, _ := repo.EnsureProvider(context.Background(), providerID, "+2348000000001")
	provider.FullName = &fullName
	provider.Email = &email
	provider.State = &state
	provider.City = &city
	provider.ProfilePhotoURL = &photo
	provider.OperationType = &op
	provider.AvgRating = 4.5
	provider.TotalTrips = 12
	repo.providers[provider.ID] = provider
	router, _ := buildProfileTestRouter(repo)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/"+providerID+"/public", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	allowed := map[string]struct{}{
		"provider_id": {}, "full_name": {}, "profile_photo_url": {},
		"verification_status": {}, "avg_rating": {}, "total_trips": {},
	}
	for field := range data {
		if _, ok := allowed[field]; !ok {
			t.Fatalf("public profile exposed non-public field %q in %s", field, w.Body.String())
		}
	}
	for _, field := range []string{"phone", "email", "state", "city", "country", "operation_type", "emergency_contact", "guarantor", "session"} {
		if _, ok := data[field]; ok {
			t.Fatalf("public profile exposed %s: %s", field, w.Body.String())
		}
	}
}

func TestRepositorySQLUsesParameterizedQueries(t *testing.T) {
	for _, path := range []string{"repository.go", "subscriber.go"} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		source := string(raw)
		for _, forbidden := range []string{
			"fmt.Sprintf(",
			"Query(ctx, fmt.",
			"QueryRow(ctx, fmt.",
			"Exec(ctx, fmt.",
			"WHERE id = '",
			"WHERE provider_id = '",
		} {
			if strings.Contains(source, forbidden) {
				t.Fatalf("%s contains unsafe SQL construction marker %q", path, forbidden)
			}
		}
	}
}

type trackingProfileRepository struct {
	*fakeProfileRepository
	getProviderCalls      int
	ensureProviderCalls   int
	updateOnboardingCalls int
	patchProviderCalls    int
	upsertContactCalls    int
	upsertGuarantorCalls  int
}

func (r *trackingProfileRepository) GetProviderByID(ctx context.Context, providerID string) (Provider, bool, error) {
	r.getProviderCalls++
	return r.fakeProfileRepository.GetProviderByID(ctx, providerID)
}

func (r *trackingProfileRepository) EnsureProvider(ctx context.Context, providerID string, phone string) (Provider, error) {
	r.ensureProviderCalls++
	return r.fakeProfileRepository.EnsureProvider(ctx, providerID, phone)
}

func (r *trackingProfileRepository) UpdateOnboarding(ctx context.Context, providerID string, input OnboardingInput) (Provider, error) {
	r.updateOnboardingCalls++
	return r.fakeProfileRepository.UpdateOnboarding(ctx, providerID, input)
}

func (r *trackingProfileRepository) PatchProvider(ctx context.Context, providerID string, input UpdateProviderInput) (Provider, error) {
	r.patchProviderCalls++
	return r.fakeProfileRepository.PatchProvider(ctx, providerID, input)
}

func (r *trackingProfileRepository) UpsertEmergencyContact(ctx context.Context, providerID string, input EmergencyContactInput) (EmergencyContact, error) {
	r.upsertContactCalls++
	return r.fakeProfileRepository.UpsertEmergencyContact(ctx, providerID, input)
}

func (r *trackingProfileRepository) UpsertGuarantor(ctx context.Context, providerID string, input GuarantorInput) (Guarantor, error) {
	r.upsertGuarantorCalls++
	return r.fakeProfileRepository.UpsertGuarantor(ctx, providerID, input)
}

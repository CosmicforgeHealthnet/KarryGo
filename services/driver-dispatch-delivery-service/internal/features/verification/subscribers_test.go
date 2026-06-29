package verification

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

const testProviderID = "11111111-1111-1111-1111-111111111111"

func TestOnboardingCompletedCreatesSixVerificationSteps(t *testing.T) {
	repo := newFakeVerificationRepository()
	payload := mustOnboardingCompletedPayload(t, testProviderID)

	if err := HandleOnboardingCompletedPayload(context.Background(), repo, payload); err != nil {
		t.Fatalf("HandleOnboardingCompletedPayload() error = %v", err)
	}

	steps, err := repo.ListSteps(context.Background(), testProviderID)
	if err != nil {
		t.Fatalf("ListSteps() error = %v", err)
	}
	if len(steps) != 6 {
		t.Fatalf("steps len = %d, want 6: %+v", len(steps), steps)
	}

	assertStepState(t, repo, StepIdentity, StatusPending, false, false)
	assertStepState(t, repo, StepLicence, StatusPending, true, false)
	assertStepState(t, repo, StepVehicle, StatusPending, false, false)
	assertStepState(t, repo, StepFace, StatusPending, false, false)
	assertStepState(t, repo, StepGuarantor, StatusApproved, false, true)
	assertStepState(t, repo, StepEmergency, StatusApproved, false, true)

	if len(repo.auditRows) != 2 {
		t.Fatalf("audit rows = %d, want 2: %+v", len(repo.auditRows), repo.auditRows)
	}
	if !repo.hasAudit(StepGuarantor, AuditActionAutoConfirmed) {
		t.Fatal("missing guarantor auto_confirmed audit row")
	}
	if !repo.hasAudit(StepEmergency, AuditActionAutoConfirmed) {
		t.Fatal("missing emergency auto_confirmed audit row")
	}
}

func TestDuplicateOnboardingCompletedDoesNotCreateDuplicateStepsOrAudit(t *testing.T) {
	repo := newFakeVerificationRepository()
	payload := mustOnboardingCompletedPayload(t, testProviderID)

	if err := HandleOnboardingCompletedPayload(context.Background(), repo, payload); err != nil {
		t.Fatalf("first HandleOnboardingCompletedPayload() error = %v", err)
	}
	if err := HandleOnboardingCompletedPayload(context.Background(), repo, payload); err != nil {
		t.Fatalf("duplicate HandleOnboardingCompletedPayload() error = %v", err)
	}

	if len(repo.steps[testProviderID]) != 6 {
		t.Fatalf("steps len = %d, want 6", len(repo.steps[testProviderID]))
	}
	if len(repo.auditRows) != 2 {
		t.Fatalf("audit rows = %d, want 2", len(repo.auditRows))
	}
}

func TestBadOnboardingCompletedPayloadReturnsErrorAndDoesNotCrash(t *testing.T) {
	repo := newFakeVerificationRepository()
	if err := HandleOnboardingCompletedPayload(context.Background(), repo, []byte(`{"provider_id":"bad"}`)); err == nil {
		t.Fatal("expected error for bad payload")
	}
	if len(repo.steps) != 0 {
		t.Fatalf("steps created for bad payload: %+v", repo.steps)
	}
}

func TestOnboardingCompletedDBErrorReturnsErrorAndDoesNotCrash(t *testing.T) {
	repo := newFakeVerificationRepository()
	repo.err = errors.New("db down")
	payload := mustOnboardingCompletedPayload(t, testProviderID)

	if err := HandleOnboardingCompletedPayload(context.Background(), repo, payload); err == nil {
		t.Fatal("expected DB error")
	}
}

func assertStepState(t *testing.T, repo *fakeVerificationRepository, step Step, status StepStatus, optional bool, auto bool) {
	t.Helper()
	result, ok, err := repo.GetStep(context.Background(), testProviderID, step)
	if err != nil {
		t.Fatalf("GetStep(%s) error = %v", step, err)
	}
	if !ok {
		t.Fatalf("step %s missing", step)
	}
	if result.Status != status || result.IsOptional != optional || result.IsAutoConfirmed != auto {
		t.Fatalf("step %s = %+v, want status=%s optional=%v auto=%v", step, result, status, optional, auto)
	}
	if auto {
		if result.ConfirmMethod == nil || *result.ConfirmMethod != ConfirmAuto {
			t.Fatalf("step %s confirm method = %v, want auto", step, result.ConfirmMethod)
		}
		if result.ReviewedAt == nil {
			t.Fatalf("step %s reviewed_at is nil, want set", step)
		}
	}
}

func mustOnboardingCompletedPayload(t *testing.T, providerID string) []byte {
	t.Helper()
	payload, err := json.Marshal(OnboardingCompletedEvent{
		Event:      "provider.onboarding.completed",
		ProviderID: providerID,
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return payload
}

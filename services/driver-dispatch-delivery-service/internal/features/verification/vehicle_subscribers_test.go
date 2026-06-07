package verification

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestVehicleVerifiedEventApprovesVehicleStepAndInsertsAudit(t *testing.T) {
	repo := seededVerificationRepo(t)

	if err := HandleVehicleVerifiedPayload(context.Background(), repo, mustVehicleVerifiedPayload(t, testProviderID)); err != nil {
		t.Fatalf("HandleVehicleVerifiedPayload() error = %v", err)
	}

	step := repo.steps[testProviderID][StepVehicle]
	if step.Status != StatusApproved {
		t.Fatalf("vehicle status = %s, want approved", step.Status)
	}
	if step.ConfirmMethod == nil || *step.ConfirmMethod != ConfirmAuto {
		t.Fatalf("confirm_method = %v, want auto", step.ConfirmMethod)
	}
	if step.ReviewedAt == nil {
		t.Fatal("reviewed_at is nil, want set")
	}
	if countAuditRows(repo, StepVehicle, AuditActionApproved) != 1 {
		t.Fatalf("vehicle approved audit rows = %d, want 1: %+v", countAuditRows(repo, StepVehicle, AuditActionApproved), repo.auditRows)
	}
}

func TestVehicleVerifiedMarksProviderVerifiedWhenRequiredStepsApproved(t *testing.T) {
	repo := seededVerificationRepo(t)
	setStepStatus(t, repo, StepIdentity, StatusApproved)
	setStepStatus(t, repo, StepFace, StatusApproved)

	if err := HandleVehicleVerifiedPayload(context.Background(), repo, mustVehicleVerifiedPayload(t, testProviderID)); err != nil {
		t.Fatalf("HandleVehicleVerifiedPayload() error = %v", err)
	}

	if repo.providerStates[testProviderID].VerificationStatus != string(OverallStatusVerified) {
		t.Fatalf("provider verification_status = %s, want verified", repo.providerStates[testProviderID].VerificationStatus)
	}
}

func TestVehicleRejectedEventRejectsVehicleStepAndInsertsAudit(t *testing.T) {
	repo := seededVerificationRepo(t)
	reason := "Document not clear"

	if err := HandleVehicleRejectedPayload(context.Background(), repo, mustVehicleRejectedPayload(t, testProviderID, reason)); err != nil {
		t.Fatalf("HandleVehicleRejectedPayload() error = %v", err)
	}

	step := repo.steps[testProviderID][StepVehicle]
	if step.Status != StatusRejected {
		t.Fatalf("vehicle status = %s, want rejected", step.Status)
	}
	if step.RejectionReason == nil || *step.RejectionReason != reason {
		t.Fatalf("rejection_reason = %v, want %q", step.RejectionReason, reason)
	}
	if step.ReviewedAt == nil {
		t.Fatal("reviewed_at is nil, want set")
	}
	if countAuditRows(repo, StepVehicle, AuditActionRejected) != 1 {
		t.Fatalf("vehicle rejected audit rows = %d, want 1: %+v", countAuditRows(repo, StepVehicle, AuditActionRejected), repo.auditRows)
	}
}

func TestBadVehiclePayloadsReturnErrorAndDoNotMutate(t *testing.T) {
	cases := []struct {
		name   string
		handle func(context.Context, Repository, []byte) error
	}{
		{name: "verified", handle: HandleVehicleVerifiedPayload},
		{name: "rejected", handle: HandleVehicleRejectedPayload},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := seededVerificationRepo(t)
			err := tc.handle(context.Background(), repo, []byte(`{"provider_id":"bad"}`))
			if err == nil {
				t.Fatal("expected error for bad payload")
			}
			if repo.steps[testProviderID][StepVehicle].Status != StatusPending {
				t.Fatalf("vehicle status = %s, want unchanged pending", repo.steps[testProviderID][StepVehicle].Status)
			}
			if countAuditRows(repo, StepVehicle, AuditActionApproved)+countAuditRows(repo, StepVehicle, AuditActionRejected) != 0 {
				t.Fatalf("vehicle audit rows were inserted for bad payload: %+v", repo.auditRows)
			}
		})
	}
}

func TestVehicleEventsIgnoreMissingStepWithoutCrashing(t *testing.T) {
	repo := newFakeVerificationRepository()
	if err := HandleVehicleVerifiedPayload(context.Background(), repo, mustVehicleVerifiedPayload(t, testProviderID)); err != nil {
		t.Fatalf("HandleVehicleVerifiedPayload() error = %v", err)
	}
	if err := HandleVehicleRejectedPayload(context.Background(), repo, mustVehicleRejectedPayload(t, testProviderID, "Missing docs")); err != nil {
		t.Fatalf("HandleVehicleRejectedPayload() error = %v", err)
	}
	if len(repo.auditRows) != 0 {
		t.Fatalf("audit rows = %+v, want none for missing step", repo.auditRows)
	}
}

func TestStatusReflectsVehicleApprovedAfterEvent(t *testing.T) {
	repo := seededVerificationRepo(t)

	if err := HandleVehicleVerifiedPayload(context.Background(), repo, mustVehicleVerifiedPayload(t, testProviderID)); err != nil {
		t.Fatalf("HandleVehicleVerifiedPayload() error = %v", err)
	}
	status, err := NewService(repo, NewStubSmileIdentityClient()).GetAllStatus(context.Background(), testProviderID)
	if err != nil {
		t.Fatalf("GetAllStatus() error = %v", err)
	}
	if status.OverallStatus != OverallStatusInProgress {
		t.Fatalf("overall_status = %s, want in_progress after manual vehicle approval", status.OverallStatus)
	}
	if findStepSummary(t, status.Steps, StepVehicle).Status != StatusApproved {
		t.Fatalf("vehicle status = %s, want approved", findStepSummary(t, status.Steps, StepVehicle).Status)
	}
}

func TestStatusReflectsVehicleRejectedAfterEvent(t *testing.T) {
	repo := seededVerificationRepo(t)

	if err := HandleVehicleRejectedPayload(context.Background(), repo, mustVehicleRejectedPayload(t, testProviderID, "Document not clear")); err != nil {
		t.Fatalf("HandleVehicleRejectedPayload() error = %v", err)
	}
	status, err := NewService(repo, NewStubSmileIdentityClient()).GetAllStatus(context.Background(), testProviderID)
	if err != nil {
		t.Fatalf("GetAllStatus() error = %v", err)
	}
	if status.OverallStatus != OverallStatusRejected {
		t.Fatalf("overall_status = %s, want rejected", status.OverallStatus)
	}
	if findStepSummary(t, status.Steps, StepVehicle).Status != StatusRejected {
		t.Fatalf("vehicle status = %s, want rejected", findStepSummary(t, status.Steps, StepVehicle).Status)
	}
}

func TestDuplicateVehicleVerifiedEventIsIdempotentForAudit(t *testing.T) {
	repo := seededVerificationRepo(t)
	payload := mustVehicleVerifiedPayload(t, testProviderID)

	if err := HandleVehicleVerifiedPayload(context.Background(), repo, payload); err != nil {
		t.Fatalf("first HandleVehicleVerifiedPayload() error = %v", err)
	}
	if err := HandleVehicleVerifiedPayload(context.Background(), repo, payload); err != nil {
		t.Fatalf("duplicate HandleVehicleVerifiedPayload() error = %v", err)
	}

	if repo.steps[testProviderID][StepVehicle].Status != StatusApproved {
		t.Fatalf("vehicle status = %s, want approved", repo.steps[testProviderID][StepVehicle].Status)
	}
	if countAuditRows(repo, StepVehicle, AuditActionApproved) != 1 {
		t.Fatalf("vehicle approved audit rows = %d, want 1: %+v", countAuditRows(repo, StepVehicle, AuditActionApproved), repo.auditRows)
	}
}

func mustVehicleVerifiedPayload(t *testing.T, providerID string) []byte {
	t.Helper()
	payload, err := json.Marshal(VehicleVerifiedEvent{
		Event:         TopicVehicleVerified,
		CorrelationID: "vehicle-test",
		ProviderID:    providerID,
		BikeID:        "22222222-2222-2222-2222-222222222222",
		CreatedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("marshal vehicle verified payload: %v", err)
	}
	return payload
}

func mustVehicleRejectedPayload(t *testing.T, providerID string, reason string) []byte {
	t.Helper()
	payload, err := json.Marshal(VehicleRejectedEvent{
		Event:         TopicVehicleRejected,
		CorrelationID: "vehicle-test",
		ProviderID:    providerID,
		BikeID:        "22222222-2222-2222-2222-222222222222",
		Reason:        reason,
		CreatedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("marshal vehicle rejected payload: %v", err)
	}
	return payload
}

func countAuditRows(repo *fakeVerificationRepository, step Step, action AuditAction) int {
	count := 0
	for _, row := range repo.auditRows {
		if row.Step == step && row.Action == action {
			count++
		}
	}
	return count
}

func findStepSummary(t *testing.T, steps []VerificationStepSummary, step Step) VerificationStepSummary {
	t.Helper()
	for _, candidate := range steps {
		if candidate.Step == step {
			return candidate
		}
	}
	t.Fatalf("step %s missing in %+v", step, steps)
	return VerificationStepSummary{}
}

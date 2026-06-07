package verification

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ── helpers ───────────────────────────────────────────────────────────────────

const (
	adminReviewTestAdminID    = "11111111-1111-1111-1111-111111111111"
	adminReviewTestProviderID = "22222222-2222-2222-2222-222222222222"
)

func initSteps(repo *fakeVerificationRepository, providerID string) {
	repo.steps[providerID] = map[Step]VerificationStep{
		StepIdentity:  {ID: "step-identity", ProviderID: providerID, Step: StepIdentity, Status: StatusSubmitted},
		StepLicence:   {ID: "step-licence", ProviderID: providerID, Step: StepLicence, Status: StatusSubmitted, IsOptional: true},
		StepVehicle:   {ID: "step-vehicle", ProviderID: providerID, Step: StepVehicle, Status: StatusSubmitted},
		StepFace:      {ID: "step-face", ProviderID: providerID, Step: StepFace, Status: StatusSubmitted},
		StepGuarantor: {ID: "step-guarantor", ProviderID: providerID, Step: StepGuarantor, Status: StatusApproved, IsAutoConfirmed: true},
		StepEmergency: {ID: "step-emergency", ProviderID: providerID, Step: StepEmergency, Status: StatusApproved, IsAutoConfirmed: true},
	}
	repo.providerStates[providerID] = ProviderVerificationState{
		ProviderID:         providerID,
		VerificationStatus: "unverified",
		IsActive:           true,
	}
}

type fakeEventPublisher struct {
	statusUpdated []VerificationStatusUpdatedEvent
	fullyApproved []VerificationFullyApprovedEvent
	rejected      []VerificationRejectedEvent
	stepSubmitted []StepSubmittedEvent
	faceFailed    []FaceFailedEvent
}

func (p *fakeEventPublisher) PublishStepSubmitted(_ context.Context, e StepSubmittedEvent) error {
	p.stepSubmitted = append(p.stepSubmitted, e)
	return nil
}

func (p *fakeEventPublisher) PublishFaceFailed(_ context.Context, e FaceFailedEvent) error {
	p.faceFailed = append(p.faceFailed, e)
	return nil
}

func (p *fakeEventPublisher) PublishVerificationStatusUpdated(_ context.Context, e VerificationStatusUpdatedEvent) error {
	p.statusUpdated = append(p.statusUpdated, e)
	return nil
}

func (p *fakeEventPublisher) PublishVerificationFullyApproved(_ context.Context, e VerificationFullyApprovedEvent) error {
	p.fullyApproved = append(p.fullyApproved, e)
	return nil
}

func (p *fakeEventPublisher) PublishVerificationRejected(_ context.Context, e VerificationRejectedEvent) error {
	p.rejected = append(p.rejected, e)
	return nil
}

func newAdminTestService(repo *fakeVerificationRepository, publisher *fakeEventPublisher) *Service {
	return NewService(repo, NewStubSmileIdentityClient(), WithEventPublisher(publisher))
}

func adminReviewHTTPRequest(
	t *testing.T,
	router http.Handler,
	token string,
	providerID string,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()
	path := "/api/v1/admin/verification/" + providerID + "/review"
	req := httptest.NewRequest(http.MethodPatch, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func mustParseData(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	return data
}

// ── Route / auth protection tests ─────────────────────────────────────────────

func TestAdminReviewReturns401WithoutJWT(t *testing.T) {
	router, _ := buildVerificationTestRouter(newFakeVerificationRepository())
	w := adminReviewHTTPRequest(t, router, "", adminReviewTestProviderID, `{}`)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401; body = %s", w.Code, w.Body.String())
	}
	assertVerificationErrorCode(t, w.Body.Bytes(), "unauthorized")
}

func TestAdminReviewReturns403ForDispatchProviderJWT(t *testing.T) {
	router, tokens := buildVerificationTestRouter(newFakeVerificationRepository())
	token, _, _ := tokens.GenerateAccessToken(adminReviewTestAdminID, "+2348000000001", "session-123")
	w := adminReviewHTTPRequest(t, router, token, adminReviewTestProviderID, `{"step":"identity","action":"approve"}`)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", w.Code, w.Body.String())
	}
	assertVerificationErrorCode(t, w.Body.Bytes(), "forbidden")
}

func TestAdminReviewAllowsPlatformAdminJWT(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	router, tokens := buildVerificationTestRouter(repo)
	token := mustRoleToken(t, tokens, RolePlatformAdmin)

	w := adminReviewHTTPRequest(t, router, token, adminReviewTestProviderID,
		`{"step":"identity","action":"approve"}`)
	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden {
		t.Fatalf("admin JWT must be allowed: status = %d; body = %s", w.Code, w.Body.String())
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
}

// ── Validation tests ──────────────────────────────────────────────────────────

func TestAdminReviewInvalidProviderIDReturns400(t *testing.T) {
	router, tokens := buildVerificationTestRouter(newFakeVerificationRepository())
	token := mustRoleToken(t, tokens, RolePlatformAdmin)

	w := adminReviewHTTPRequest(t, router, token, "not-a-uuid",
		`{"step":"identity","action":"approve"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	assertVerificationErrorCode(t, w.Body.Bytes(), "validation_failed")
}

func TestAdminReviewMissingStepReturns400(t *testing.T) {
	repo := newFakeVerificationRepository()
	router, tokens := buildVerificationTestRouter(repo)
	token := mustRoleToken(t, tokens, RolePlatformAdmin)

	w := adminReviewHTTPRequest(t, router, token, adminReviewTestProviderID,
		`{"action":"approve"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	assertVerificationErrorCode(t, w.Body.Bytes(), "validation_failed")
}

func TestAdminReviewInvalidStepReturns400(t *testing.T) {
	repo := newFakeVerificationRepository()
	router, tokens := buildVerificationTestRouter(repo)
	token := mustRoleToken(t, tokens, RolePlatformAdmin)

	w := adminReviewHTTPRequest(t, router, token, adminReviewTestProviderID,
		`{"step":"unknown","action":"approve"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	assertVerificationErrorCode(t, w.Body.Bytes(), "validation_failed")
}

func TestAdminReviewMissingActionReturns400(t *testing.T) {
	repo := newFakeVerificationRepository()
	router, tokens := buildVerificationTestRouter(repo)
	token := mustRoleToken(t, tokens, RolePlatformAdmin)

	w := adminReviewHTTPRequest(t, router, token, adminReviewTestProviderID,
		`{"step":"identity"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	assertVerificationErrorCode(t, w.Body.Bytes(), "validation_failed")
}

func TestAdminReviewInvalidActionReturns400(t *testing.T) {
	repo := newFakeVerificationRepository()
	router, tokens := buildVerificationTestRouter(repo)
	token := mustRoleToken(t, tokens, RolePlatformAdmin)

	w := adminReviewHTTPRequest(t, router, token, adminReviewTestProviderID,
		`{"step":"identity","action":"maybe"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	assertVerificationErrorCode(t, w.Body.Bytes(), "validation_failed")
}

func TestAdminReviewRejectWithoutReasonReturns400(t *testing.T) {
	repo := newFakeVerificationRepository()
	router, tokens := buildVerificationTestRouter(repo)
	token := mustRoleToken(t, tokens, RolePlatformAdmin)

	w := adminReviewHTTPRequest(t, router, token, adminReviewTestProviderID,
		`{"step":"identity","action":"reject"}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
	}
	assertVerificationErrorCode(t, w.Body.Bytes(), "validation_failed")
}

func TestAdminReviewMissingStepRowReturns404(t *testing.T) {
	repo := newFakeVerificationRepository()
	// No steps initialized for adminReviewTestProviderID
	router, tokens := buildVerificationTestRouter(repo)
	token := mustRoleToken(t, tokens, RolePlatformAdmin)

	w := adminReviewHTTPRequest(t, router, token, adminReviewTestProviderID,
		`{"step":"identity","action":"approve"}`)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
	assertVerificationErrorCode(t, w.Body.Bytes(), "not_found")
}

// ── Approve behavior tests ────────────────────────────────────────────────────

func TestAdminReviewApproveIdentitySetsStatusApproved(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	router, tokens := buildVerificationTestRouter(repo)
	token := mustRoleToken(t, tokens, RolePlatformAdmin)

	w := adminReviewHTTPRequest(t, router, token, adminReviewTestProviderID,
		`{"step":"identity","action":"approve"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	data := mustParseData(t, w.Body.Bytes())
	if data["status"] != "approved" {
		t.Fatalf("status = %v, want approved", data["status"])
	}
	if data["step"] != "identity" {
		t.Fatalf("step = %v, want identity", data["step"])
	}
}

func TestAdminReviewApproveIdentitySetsReviewerID(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	router, tokens := buildVerificationTestRouter(repo)
	token := mustRoleToken(t, tokens, RolePlatformAdmin)

	w := adminReviewHTTPRequest(t, router, token, adminReviewTestProviderID,
		`{"step":"identity","action":"approve"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	data := mustParseData(t, w.Body.Bytes())
	if data["reviewer_id"] == nil || data["reviewer_id"] == "" {
		t.Fatalf("reviewer_id not set in response: %v", data)
	}
}

func TestAdminReviewApproveIdentitySetsConfirmMethodManual(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	svc := newAdminTestService(repo, &fakeEventPublisher{})
	_, err := svc.AdminReview(context.Background(), adminReviewTestAdminID, "corr-1", adminReviewTestProviderID, AdminReviewRequest{
		Step:   StepIdentity,
		Action: AdminActionApprove,
	})
	if err != nil {
		t.Fatalf("AdminReview() error = %v", err)
	}
	step := repo.steps[adminReviewTestProviderID][StepIdentity]
	if step.ConfirmMethod == nil || *step.ConfirmMethod != ConfirmManual {
		t.Fatalf("confirm_method = %v, want manual", step.ConfirmMethod)
	}
}

func TestAdminReviewApproveIdentityInsertsAuditRow(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	svc := newAdminTestService(repo, &fakeEventPublisher{})
	_, err := svc.AdminReview(context.Background(), adminReviewTestAdminID, "corr-1", adminReviewTestProviderID, AdminReviewRequest{
		Step:   StepIdentity,
		Action: AdminActionApprove,
	})
	if err != nil {
		t.Fatalf("AdminReview() error = %v", err)
	}
	var found bool
	for _, row := range repo.auditRows {
		if row.Step == StepIdentity && row.Action == AuditActionApproved {
			found = true
			if row.ToStatus != StatusApproved {
				t.Fatalf("audit to_status = %s, want approved", row.ToStatus)
			}
			break
		}
	}
	if !found {
		t.Fatalf("no audit row found for identity approve")
	}
}

// ── Reject behavior tests ─────────────────────────────────────────────────────

func TestAdminReviewRejectIdentitySetsStatusRejected(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	router, tokens := buildVerificationTestRouter(repo)
	token := mustRoleToken(t, tokens, RolePlatformAdmin)

	w := adminReviewHTTPRequest(t, router, token, adminReviewTestProviderID,
		`{"step":"identity","action":"reject","reason":"Document is not clear"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	data := mustParseData(t, w.Body.Bytes())
	if data["status"] != "rejected" {
		t.Fatalf("status = %v, want rejected", data["status"])
	}
	if data["rejection_reason"] != "Document is not clear" {
		t.Fatalf("rejection_reason = %v", data["rejection_reason"])
	}
}

func TestAdminReviewRejectIdentityStoresRejectionReason(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	svc := newAdminTestService(repo, &fakeEventPublisher{})
	_, err := svc.AdminReview(context.Background(), adminReviewTestAdminID, "corr-1", adminReviewTestProviderID, AdminReviewRequest{
		Step:   StepIdentity,
		Action: AdminActionReject,
		Reason: "Too blurry",
	})
	if err != nil {
		t.Fatalf("AdminReview() error = %v", err)
	}
	step := repo.steps[adminReviewTestProviderID][StepIdentity]
	if step.RejectionReason == nil || *step.RejectionReason != "Too blurry" {
		t.Fatalf("rejection_reason = %v, want 'Too blurry'", step.RejectionReason)
	}
}

func TestAdminReviewRejectIdentityInsertsAuditRow(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	svc := newAdminTestService(repo, &fakeEventPublisher{})
	_, err := svc.AdminReview(context.Background(), adminReviewTestAdminID, "corr-1", adminReviewTestProviderID, AdminReviewRequest{
		Step:   StepIdentity,
		Action: AdminActionReject,
		Reason: "Too blurry",
	})
	if err != nil {
		t.Fatalf("AdminReview() error = %v", err)
	}
	var found bool
	for _, row := range repo.auditRows {
		if row.Step == StepIdentity && row.Action == AuditActionRejected {
			found = true
			if row.ToStatus != StatusRejected {
				t.Fatalf("audit to_status = %s, want rejected", row.ToStatus)
			}
			break
		}
	}
	if !found {
		t.Fatalf("no audit row found for identity reject")
	}
}

// ── Conflict test ─────────────────────────────────────────────────────────────

func TestAdminReviewAlreadyApprovedReturns409(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	// Mark identity as already approved.
	step := repo.steps[adminReviewTestProviderID][StepIdentity]
	step.Status = StatusApproved
	repo.steps[adminReviewTestProviderID][StepIdentity] = step

	router, tokens := buildVerificationTestRouter(repo)
	token := mustRoleToken(t, tokens, RolePlatformAdmin)

	w := adminReviewHTTPRequest(t, router, token, adminReviewTestProviderID,
		`{"step":"identity","action":"approve"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", w.Code, w.Body.String())
	}
	assertVerificationErrorCode(t, w.Body.Bytes(), "conflict")
}

// ── Event tests ───────────────────────────────────────────────────────────────

func TestAdminReviewPublishesStatusUpdatedOnApprove(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	pub := &fakeEventPublisher{}
	svc := newAdminTestService(repo, pub)

	_, err := svc.AdminReview(context.Background(), adminReviewTestAdminID, "corr-1", adminReviewTestProviderID, AdminReviewRequest{
		Step:   StepIdentity,
		Action: AdminActionApprove,
	})
	if err != nil {
		t.Fatalf("AdminReview() error = %v", err)
	}
	if len(pub.statusUpdated) == 0 {
		t.Fatal("verification.status.updated not published")
	}
	found := false
	for _, ev := range pub.statusUpdated {
		if ev.Step == StepIdentity && ev.Status == StatusApproved {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no status_updated event with step=identity status=approved; got %+v", pub.statusUpdated)
	}
}

func TestAdminReviewPublishesVerificationRejectedOnReject(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	pub := &fakeEventPublisher{}
	svc := newAdminTestService(repo, pub)

	_, err := svc.AdminReview(context.Background(), adminReviewTestAdminID, "corr-1", adminReviewTestProviderID, AdminReviewRequest{
		Step:   StepFace,
		Action: AdminActionReject,
		Reason: "Selfie too dark",
	})
	if err != nil {
		t.Fatalf("AdminReview() error = %v", err)
	}
	if len(pub.rejected) != 1 {
		t.Fatalf("verification.rejected published %d times, want 1", len(pub.rejected))
	}
	if pub.rejected[0].Step != StepFace || pub.rejected[0].Reason != "Selfie too dark" {
		t.Fatalf("rejected event = %+v", pub.rejected[0])
	}
}

func TestAdminReviewFullyApprovedFiresEventOnlyWhenAllRequired(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	pub := &fakeEventPublisher{}
	svc := newAdminTestService(repo, pub)
	ctx := context.Background()

	// Approve identity — not yet fully approved (vehicle and face still submitted)
	if _, err := svc.AdminReview(ctx, adminReviewTestAdminID, "c1", adminReviewTestProviderID, AdminReviewRequest{
		Step: StepIdentity, Action: AdminActionApprove,
	}); err != nil {
		t.Fatalf("approve identity: %v", err)
	}
	if len(pub.fullyApproved) != 0 {
		t.Fatalf("fully_approved fired too early after identity: %d events", len(pub.fullyApproved))
	}

	// Approve face
	if _, err := svc.AdminReview(ctx, adminReviewTestAdminID, "c2", adminReviewTestProviderID, AdminReviewRequest{
		Step: StepFace, Action: AdminActionApprove,
	}); err != nil {
		t.Fatalf("approve face: %v", err)
	}
	if len(pub.fullyApproved) != 0 {
		t.Fatalf("fully_approved fired too early after face: %d events", len(pub.fullyApproved))
	}

	// Approve vehicle (via fake direct state — normally from vehicle event, but admin can't do it)
	step := repo.steps[adminReviewTestProviderID][StepVehicle]
	step.Status = StatusApproved
	repo.steps[adminReviewTestProviderID][StepVehicle] = step

	// Now all required steps approved: identity✓ vehicle✓ face✓ guarantor✓(auto) emergency✓(auto)
	// Approve licence (optional) — should trigger the full-approval check via another approve action
	// Actually, let me trigger it differently: run markProviderVerifiedIfComplete via AdminReview on face again... No.
	// We need to approve something that re-triggers the check. Face is already approved → can't re-approve.
	// Licence is optional, so approving it will trigger checkRequiredStepsApproved.
	if _, err := svc.AdminReview(ctx, adminReviewTestAdminID, "c3", adminReviewTestProviderID, AdminReviewRequest{
		Step: StepLicence, Action: AdminActionApprove,
	}); err != nil {
		t.Fatalf("approve licence: %v", err)
	}
	if len(pub.fullyApproved) != 1 {
		t.Fatalf("fully_approved fired %d times after all required approved, want 1", len(pub.fullyApproved))
	}
}

func TestAdminReviewLicenceOptionalDoesNotBlockFullApproval(t *testing.T) {
	// When all required steps approved but licence is still pending/submitted,
	// CheckRequiredStepsApproved should still return true.
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	// Manually set all required steps to approved.
	for _, s := range []Step{StepIdentity, StepVehicle, StepFace, StepGuarantor, StepEmergency} {
		step := repo.steps[adminReviewTestProviderID][s]
		step.Status = StatusApproved
		repo.steps[adminReviewTestProviderID][s] = step
	}
	// Licence stays submitted.
	approved, err := repo.CheckRequiredStepsApproved(context.Background(), adminReviewTestProviderID)
	if err != nil {
		t.Fatalf("CheckRequiredStepsApproved() error = %v", err)
	}
	if !approved {
		t.Fatal("licence pending should not block full approval")
	}
}

func TestAdminReviewVehiclePendingBlocksFullApproval(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	// Set identity and face approved, vehicle still submitted.
	for _, s := range []Step{StepIdentity, StepFace, StepGuarantor, StepEmergency} {
		step := repo.steps[adminReviewTestProviderID][s]
		step.Status = StatusApproved
		repo.steps[adminReviewTestProviderID][s] = step
	}
	approved, err := repo.CheckRequiredStepsApproved(context.Background(), adminReviewTestProviderID)
	if err != nil {
		t.Fatalf("CheckRequiredStepsApproved() error = %v", err)
	}
	if approved {
		t.Fatal("vehicle pending should block full approval")
	}
}

func TestAdminReviewFaceSubmittedBlocksFullApproval(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	for _, s := range []Step{StepIdentity, StepVehicle, StepGuarantor, StepEmergency} {
		step := repo.steps[adminReviewTestProviderID][s]
		step.Status = StatusApproved
		repo.steps[adminReviewTestProviderID][s] = step
	}
	// face still submitted
	approved, err := repo.CheckRequiredStepsApproved(context.Background(), adminReviewTestProviderID)
	if err != nil {
		t.Fatalf("CheckRequiredStepsApproved() error = %v", err)
	}
	if approved {
		t.Fatal("face submitted (not approved) should block full approval")
	}
}

func TestAdminReviewGuarantorAutoApprovedCountsTowardApproval(t *testing.T) {
	// Guarantor and emergency are already approved (auto) from initSteps.
	// Verify that CheckRequiredStepsApproved correctly counts them.
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	for _, s := range []Step{StepIdentity, StepVehicle, StepFace} {
		step := repo.steps[adminReviewTestProviderID][s]
		step.Status = StatusApproved
		repo.steps[adminReviewTestProviderID][s] = step
	}
	approved, err := repo.CheckRequiredStepsApproved(context.Background(), adminReviewTestProviderID)
	if err != nil {
		t.Fatalf("CheckRequiredStepsApproved() error = %v", err)
	}
	if !approved {
		t.Fatal("auto-approved guarantor and emergency should count toward full approval")
	}
}

// ── Manual-review scope tests ─────────────────────────────────────────────────

func TestAdminReviewVehicleStepIsBlocked(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	svc := newAdminTestService(repo, &fakeEventPublisher{})

	_, err := svc.AdminReview(context.Background(), adminReviewTestAdminID, "c", adminReviewTestProviderID, AdminReviewRequest{
		Step:   StepVehicle,
		Action: AdminActionApprove,
	})
	if err == nil {
		t.Fatal("expected error for vehicle step, got nil")
	}
}

func TestAdminReviewGuarantorStepIsBlocked(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	svc := newAdminTestService(repo, &fakeEventPublisher{})

	_, err := svc.AdminReview(context.Background(), adminReviewTestAdminID, "c", adminReviewTestProviderID, AdminReviewRequest{
		Step:   StepGuarantor,
		Action: AdminActionApprove,
	})
	if err == nil {
		t.Fatal("expected error for guarantor step, got nil")
	}
}

func TestAdminReviewEmergencyStepIsBlocked(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	svc := newAdminTestService(repo, &fakeEventPublisher{})

	_, err := svc.AdminReview(context.Background(), adminReviewTestAdminID, "c", adminReviewTestProviderID, AdminReviewRequest{
		Step:   StepEmergency,
		Action: AdminActionApprove,
	})
	if err == nil {
		t.Fatal("expected error for emergency step, got nil")
	}
}

// ── Existing status endpoint still works ──────────────────────────────────────

func TestExistingGetStatusEndpointStillWorks(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	router, tokens := buildVerificationTestRouter(repo)
	token, _, _ := tokens.GenerateAccessToken(adminReviewTestProviderID, "+2348000000001", "session-1")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/provider/verification/status", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /status status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
}

// ── Service-level validation unit tests ──────────────────────────────────────

func TestValidateAdminReviewInputMissingStep(t *testing.T) {
	err := validateAdminReviewInput(AdminReviewRequest{Action: AdminActionApprove})
	if err == nil {
		t.Fatal("want error for missing step")
	}
}

func TestValidateAdminReviewInputInvalidAction(t *testing.T) {
	err := validateAdminReviewInput(AdminReviewRequest{Step: StepIdentity, Action: "maybe"})
	if err == nil {
		t.Fatal("want error for invalid action")
	}
}

func TestValidateAdminReviewInputRejectRequiresReason(t *testing.T) {
	err := validateAdminReviewInput(AdminReviewRequest{Step: StepIdentity, Action: AdminActionReject})
	if err == nil {
		t.Fatal("want error for missing reject reason")
	}
}

func TestValidateAdminReviewInputApproveReasonIsOptional(t *testing.T) {
	err := validateAdminReviewInput(AdminReviewRequest{Step: StepIdentity, Action: AdminActionApprove})
	if err != nil {
		t.Fatalf("approve with no reason should be valid: %v", err)
	}
}

func TestAdminReviewPublishesStatusUpdatedWithCorrectTopic(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	pub := &fakeEventPublisher{}
	svc := newAdminTestService(repo, pub)

	_, err := svc.AdminReview(context.Background(), adminReviewTestAdminID, "corr-123", adminReviewTestProviderID, AdminReviewRequest{
		Step:   StepIdentity,
		Action: AdminActionApprove,
	})
	if err != nil {
		t.Fatalf("AdminReview() error = %v", err)
	}
	if len(pub.statusUpdated) == 0 {
		t.Fatal("no status.updated event")
	}
	if pub.statusUpdated[0].Event != TopicVerificationStatusUpdated {
		t.Fatalf("event topic = %s, want %s", pub.statusUpdated[0].Event, TopicVerificationStatusUpdated)
	}
	if pub.statusUpdated[0].CorrelationID != "corr-123" {
		t.Fatalf("correlation_id = %s, want corr-123", pub.statusUpdated[0].CorrelationID)
	}
	if pub.statusUpdated[0].ProviderID != adminReviewTestProviderID {
		t.Fatalf("provider_id = %s", pub.statusUpdated[0].ProviderID)
	}
}

// ── validateAdminReviewInput edge cases ──────────────────────────────────────

func TestValidateAdminReviewInputAllValid(t *testing.T) {
	cases := []struct {
		name  string
		input AdminReviewRequest
	}{
		{"approve identity no reason", AdminReviewRequest{Step: StepIdentity, Action: AdminActionApprove}},
		{"approve licence no reason", AdminReviewRequest{Step: StepLicence, Action: AdminActionApprove}},
		{"approve face no reason", AdminReviewRequest{Step: StepFace, Action: AdminActionApprove}},
		{"reject identity with reason", AdminReviewRequest{Step: StepIdentity, Action: AdminActionReject, Reason: "Bad doc"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateAdminReviewInput(tc.input); err != nil {
				t.Fatalf("%s: unexpected error = %v", tc.name, err)
			}
		})
	}
}

// ── Response shape test ───────────────────────────────────────────────────────

func TestAdminReviewApproveResponseShape(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	router, tokens := buildVerificationTestRouter(repo)
	token := mustRoleToken(t, tokens, RolePlatformAdmin)

	w := adminReviewHTTPRequest(t, router, token, adminReviewTestProviderID,
		`{"step":"face","action":"approve"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", w.Code, w.Body.String())
	}
	data := mustParseData(t, w.Body.Bytes())
	for _, field := range []string{"step", "status", "reviewed_at", "reviewer_id"} {
		if _, ok := data[field]; !ok {
			t.Fatalf("response missing field %s: %s", field, w.Body.String())
		}
	}
}

func TestAdminReviewRejectResponseContainsRejectionReason(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	router, tokens := buildVerificationTestRouter(repo)
	token := mustRoleToken(t, tokens, RolePlatformAdmin)

	w := adminReviewHTTPRequest(t, router, token, adminReviewTestProviderID,
		`{"step":"identity","action":"reject","reason":"ID expired"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", w.Code, w.Body.String())
	}
	data := mustParseData(t, w.Body.Bytes())
	if data["rejection_reason"] != "ID expired" {
		t.Fatalf("rejection_reason = %v, want 'ID expired'", data["rejection_reason"])
	}
}

// Keep the old test working (now returns 200 not 501 since AdminReview is implemented)
func TestAdminVerificationReviewRouteAllowsPlatformAdminJWT(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, "11111111-1111-1111-1111-111111111111")
	router, tokens := buildVerificationTestRouter(repo)
	token := mustRoleToken(t, tokens, RolePlatformAdmin)

	req := httptest.NewRequest(http.MethodPatch,
		"/api/v1/admin/verification/11111111-1111-1111-1111-111111111111/review",
		strings.NewReader(`{"step":"identity","action":"approve"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code == http.StatusUnauthorized || w.Code == http.StatusForbidden || w.Code == http.StatusNotFound {
		t.Fatalf("status = %d, want route to pass auth; body = %s", w.Code, w.Body.String())
	}
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
}

// ── time import usage ─────────────────────────────────────────────────────────

var _ = time.Now // ensure time import used if tests only reference it indirectly

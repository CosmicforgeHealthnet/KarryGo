package verification

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"os"
	"strings"
	"testing"
)

// ──────────────────────────────────────────────────────────────────────────────
// Shared helpers
// ──────────────────────────────────────────────────────────────────────────────

func fullyApproveAllRequired(repo *fakeVerificationRepository, providerID string) {
	for _, s := range requiredVerificationSteps() {
		step := repo.steps[providerID][s]
		step.Status = StatusApproved
		repo.steps[providerID][s] = step
	}
}

func allStepsNotApprovedExcept(repo *fakeVerificationRepository, providerID string, approved Step) {
	initSteps(repo, providerID)
	for _, s := range requiredVerificationSteps() {
		status := StatusSubmitted
		if s == approved {
			status = StatusApproved
		}
		step := repo.steps[providerID][s]
		step.Status = status
		repo.steps[providerID][s] = step
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Phase 3J: Fully-approved gate
// ──────────────────────────────────────────────────────────────────────────────

func TestGateDoesNotFireWhenIdentityPending(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	pub := &fakeEventPublisher{}
	svc := newAdminTestService(repo, pub)

	// All approved except identity
	for _, s := range []Step{StepVehicle, StepFace, StepGuarantor, StepEmergency} {
		step := repo.steps[adminReviewTestProviderID][s]
		step.Status = StatusApproved
		repo.steps[adminReviewTestProviderID][s] = step
	}

	if err := svc.markProviderVerifiedIfComplete(context.Background(), adminReviewTestProviderID, "c1"); err != nil {
		t.Fatalf("markProviderVerifiedIfComplete() error = %v", err)
	}
	if len(pub.fullyApproved) != 0 {
		t.Fatalf("gate fired with identity pending: %d events", len(pub.fullyApproved))
	}
}

func TestGateDoesNotFireWhenFaceSubmittedNotApproved(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	pub := &fakeEventPublisher{}
	svc := newAdminTestService(repo, pub)

	for _, s := range []Step{StepIdentity, StepVehicle, StepGuarantor, StepEmergency} {
		step := repo.steps[adminReviewTestProviderID][s]
		step.Status = StatusApproved
		repo.steps[adminReviewTestProviderID][s] = step
	}
	// face stays submitted

	if err := svc.markProviderVerifiedIfComplete(context.Background(), adminReviewTestProviderID, "c1"); err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(pub.fullyApproved) != 0 {
		t.Fatalf("gate fired with face submitted-not-approved: %d events", len(pub.fullyApproved))
	}
}

func TestGateDoesNotFireWhenVehiclePending(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	pub := &fakeEventPublisher{}
	svc := newAdminTestService(repo, pub)

	for _, s := range []Step{StepIdentity, StepFace, StepGuarantor, StepEmergency} {
		step := repo.steps[adminReviewTestProviderID][s]
		step.Status = StatusApproved
		repo.steps[adminReviewTestProviderID][s] = step
	}
	// vehicle stays submitted (pending = StatusSubmitted in test)

	if err := svc.markProviderVerifiedIfComplete(context.Background(), adminReviewTestProviderID, "c1"); err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(pub.fullyApproved) != 0 {
		t.Fatalf("gate fired with vehicle pending: %d events", len(pub.fullyApproved))
	}
}

func TestGateDoesNotFireWhenVehicleRejected(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	pub := &fakeEventPublisher{}
	svc := newAdminTestService(repo, pub)

	for _, s := range []Step{StepIdentity, StepFace, StepGuarantor, StepEmergency} {
		step := repo.steps[adminReviewTestProviderID][s]
		step.Status = StatusApproved
		repo.steps[adminReviewTestProviderID][s] = step
	}
	step := repo.steps[adminReviewTestProviderID][StepVehicle]
	step.Status = StatusRejected
	repo.steps[adminReviewTestProviderID][StepVehicle] = step

	if err := svc.markProviderVerifiedIfComplete(context.Background(), adminReviewTestProviderID, "c1"); err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(pub.fullyApproved) != 0 {
		t.Fatalf("gate fired with vehicle rejected: %d events", len(pub.fullyApproved))
	}
}

func TestGateFiresWhenAllRequiredApproved(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	pub := &fakeEventPublisher{}
	svc := newAdminTestService(repo, pub)

	fullyApproveAllRequired(repo, adminReviewTestProviderID)

	if err := svc.markProviderVerifiedIfComplete(context.Background(), adminReviewTestProviderID, "corr-1"); err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(pub.fullyApproved) != 1 {
		t.Fatalf("gate fired %d times, want 1", len(pub.fullyApproved))
	}
	if pub.fullyApproved[0].ProviderID != adminReviewTestProviderID {
		t.Fatalf("provider_id = %s", pub.fullyApproved[0].ProviderID)
	}
}

func TestGateLicencePendingDoesNotBlockApproval(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	pub := &fakeEventPublisher{}
	svc := newAdminTestService(repo, pub)

	// Required steps all approved, licence stays submitted (optional)
	fullyApproveAllRequired(repo, adminReviewTestProviderID)
	step := repo.steps[adminReviewTestProviderID][StepLicence]
	step.Status = StatusSubmitted
	repo.steps[adminReviewTestProviderID][StepLicence] = step

	if err := svc.markProviderVerifiedIfComplete(context.Background(), adminReviewTestProviderID, "corr-1"); err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(pub.fullyApproved) != 1 {
		t.Fatalf("gate blocked by optional licence: %d events fired", len(pub.fullyApproved))
	}
}

func TestGatePublishesFullyApprovedExactlyOnce(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	pub := &fakeEventPublisher{}
	svc := newAdminTestService(repo, pub)
	fullyApproveAllRequired(repo, adminReviewTestProviderID)

	// Call gate 3 times — should only fire once.
	for i := 0; i < 3; i++ {
		if err := svc.markProviderVerifiedIfComplete(context.Background(), adminReviewTestProviderID, "corr"); err != nil {
			t.Fatalf("call %d error = %v", i, err)
		}
	}
	if len(pub.fullyApproved) != 1 {
		t.Fatalf("gate fired %d times, want exactly 1", len(pub.fullyApproved))
	}
}

func TestGateInsertsFullyApprovedAuditRowExactlyOnce(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	pub := &fakeEventPublisher{}
	svc := newAdminTestService(repo, pub)
	fullyApproveAllRequired(repo, adminReviewTestProviderID)

	for i := 0; i < 3; i++ {
		if err := svc.markProviderVerifiedIfComplete(context.Background(), adminReviewTestProviderID, "corr"); err != nil {
			t.Fatalf("call %d error = %v", i, err)
		}
	}

	var count int
	for _, row := range repo.auditRows {
		if row.ProviderID == adminReviewTestProviderID && row.Action == AuditActionFullyApproved {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("fully_approved audit rows = %d, want 1", count)
	}
}

func TestGateUpdatesProviderVerificationStatusToVerified(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	pub := &fakeEventPublisher{}
	svc := newAdminTestService(repo, pub)
	fullyApproveAllRequired(repo, adminReviewTestProviderID)

	if err := svc.markProviderVerifiedIfComplete(context.Background(), adminReviewTestProviderID, "corr"); err != nil {
		t.Fatalf("error = %v", err)
	}
	state := repo.providerStates[adminReviewTestProviderID]
	if state.VerificationStatus != string(OverallStatusVerified) {
		t.Fatalf("verification_status = %s, want verified", state.VerificationStatus)
	}
}

func TestGateFullyApprovedEventHasApprovedAt(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	pub := &fakeEventPublisher{}
	svc := newAdminTestService(repo, pub)
	fullyApproveAllRequired(repo, adminReviewTestProviderID)

	if err := svc.markProviderVerifiedIfComplete(context.Background(), adminReviewTestProviderID, "corr"); err != nil {
		t.Fatalf("error = %v", err)
	}
	if pub.fullyApproved[0].ApprovedAt.IsZero() {
		t.Fatal("approved_at must not be zero")
	}
}

func TestGateVehicleVerifiedEventTriggersGate(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	pub := &fakeEventPublisher{}
	svc := newAdminTestService(repo, pub)

	// Manually approve all non-vehicle required steps.
	for _, s := range []Step{StepIdentity, StepFace, StepGuarantor, StepEmergency} {
		step := repo.steps[adminReviewTestProviderID][s]
		step.Status = StatusApproved
		repo.steps[adminReviewTestProviderID][s] = step
	}
	// Vehicle starts as submitted; ApplyVehicleVerified will approve it and run gate.
	if err := svc.ApplyVehicleVerified(context.Background(), VehicleVerifiedEvent{
		ProviderID:    adminReviewTestProviderID,
		BikeID:        "bike-001",
		CorrelationID: "corr-vehicle",
	}); err != nil {
		t.Fatalf("ApplyVehicleVerified() error = %v", err)
	}
	if len(pub.fullyApproved) != 1 {
		t.Fatalf("gate fired %d times via vehicle.verified, want 1", len(pub.fullyApproved))
	}
}

func TestGateAdminApproveTriggersGate(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	pub := &fakeEventPublisher{}
	svc := newAdminTestService(repo, pub)

	// Manually approve all except identity (which admin will approve).
	for _, s := range []Step{StepVehicle, StepFace, StepGuarantor, StepEmergency} {
		step := repo.steps[adminReviewTestProviderID][s]
		step.Status = StatusApproved
		repo.steps[adminReviewTestProviderID][s] = step
	}

	_, err := svc.AdminReview(context.Background(), adminReviewTestAdminID, "corr", adminReviewTestProviderID, AdminReviewRequest{
		Step:   StepIdentity,
		Action: AdminActionApprove,
	})
	if err != nil {
		t.Fatalf("AdminReview() error = %v", err)
	}
	if len(pub.fullyApproved) != 1 {
		t.Fatalf("gate fired %d times via admin approve, want 1", len(pub.fullyApproved))
	}
}

func TestGateDuplicateCallsDoNotDuplicateAuditOrEvent(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	pub := &fakeEventPublisher{}
	svc := newAdminTestService(repo, pub)
	fullyApproveAllRequired(repo, adminReviewTestProviderID)

	for i := 0; i < 5; i++ {
		if err := svc.markProviderVerifiedIfComplete(context.Background(), adminReviewTestProviderID, "corr"); err != nil {
			t.Fatalf("call %d error = %v", i, err)
		}
	}
	// Exactly one event
	if len(pub.fullyApproved) != 1 {
		t.Fatalf("events = %d, want 1", len(pub.fullyApproved))
	}
	// Exactly one audit row
	var cnt int
	for _, row := range repo.auditRows {
		if row.Action == AuditActionFullyApproved {
			cnt++
		}
	}
	if cnt != 1 {
		t.Fatalf("fully_approved audit rows = %d, want 1", cnt)
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Phase 3K: Events
// ──────────────────────────────────────────────────────────────────────────────

func TestStepSubmittedEventPublishesForIdentity(t *testing.T) {
	repo := seededVerificationRepo(t)
	events := &captureEvents{}
	svc := NewService(repo, NewStubSmileIdentityClient(), WithEventPublisher(events))

	header := stubFileHeader{filename: "id.pdf", contentType: "application/pdf", content: testPDF}
	photoHeader := stubFileHeader{filename: "me.jpg", contentType: "image/jpeg", content: testJPEG}
	uploader := &captureUploader{}

	svc.uploader = uploader
	_, err := svc.SubmitIdentity(context.Background(), IdentitySubmissionInput{
		ProviderID:    testProviderID,
		CorrelationID: "corr-id",
		GovtIDType:    "nin",
		GovtIDNumber:  "12345678901",
		GovtIDFile:    FileUpload{Header: header},
		ProfilePhoto:  FileUpload{Header: photoHeader},
	})
	if err != nil {
		t.Fatalf("SubmitIdentity() error = %v", err)
	}
	if len(events.stepSubmitted) != 1 || events.stepSubmitted[0].Step != StepIdentity {
		t.Fatalf("stepSubmitted = %+v", events.stepSubmitted)
	}
}

func TestStepSubmittedEventPublishesForLicence(t *testing.T) {
	repo := seededVerificationRepo(t)
	events := &captureEvents{}
	svc := NewService(repo, NewStubSmileIdentityClient(), WithEventPublisher(events))
	svc.uploader = &captureUploader{}

	header := stubFileHeader{filename: "lic.pdf", contentType: "application/pdf", content: testPDF}
	_, err := svc.SubmitLicence(context.Background(), LicenceSubmissionInput{
		ProviderID:    testProviderID,
		CorrelationID: "corr-lic",
		LicenceNumber: "DRV123",
		ExpiryYear:    "2028",
		ExpiryMonth:   "06",
		LicenceFile:   FileUpload{Header: header},
	})
	if err != nil {
		t.Fatalf("SubmitLicence() error = %v", err)
	}
	if len(events.stepSubmitted) != 1 || events.stepSubmitted[0].Step != StepLicence {
		t.Fatalf("stepSubmitted = %+v", events.stepSubmitted)
	}
}

func seededRepoWithIdentitySubmitted(t *testing.T) *fakeVerificationRepository {
	t.Helper()
	repo := seededVerificationRepo(t)
	// Mark identity as submitted so face submission is allowed.
	step := repo.steps[testProviderID][StepIdentity]
	step.Status = StatusSubmitted
	repo.steps[testProviderID][StepIdentity] = step
	// Add a govt_id document so face check can reference it.
	repo.documents = append(repo.documents, VerificationDocument{
		ID:           "doc-govtid",
		StepID:       "step-" + string(StepIdentity),
		ProviderID:   testProviderID,
		DocumentType: "govt_id",
		FileURL:      "local-private://verifications/test/identity/id.pdf",
	})
	return repo
}

func TestStepSubmittedEventPublishesForFacePass(t *testing.T) {
	repo := seededRepoWithIdentitySubmitted(t)
	events := &captureEvents{}
	svc := NewService(repo, &fakeFaceMatcher{score: 95.0, passed: true}, WithEventPublisher(events))
	svc.uploader = &captureUploader{}

	header := stubFileHeader{filename: "selfie.jpg", contentType: "image/jpeg", content: testJPEG}
	_, err := svc.SubmitFace(context.Background(), FaceSubmissionInput{
		ProviderID:    testProviderID,
		CorrelationID: "corr-face",
		Selfie:        FileUpload{Header: header},
	})
	if err != nil {
		t.Fatalf("SubmitFace() error = %v", err)
	}
	if len(events.stepSubmitted) != 1 || events.stepSubmitted[0].Step != StepFace {
		t.Fatalf("stepSubmitted = %+v, want face submitted", events.stepSubmitted)
	}
}

func TestFaceFailedEventPublishesForFaceMismatch(t *testing.T) {
	repo := seededRepoWithIdentitySubmitted(t)
	events := &captureEvents{}
	svc := NewService(repo, &fakeFaceMatcher{score: 20.0, passed: false}, WithEventPublisher(events))
	svc.uploader = &captureUploader{}

	header := stubFileHeader{filename: "selfie.jpg", contentType: "image/jpeg", content: testJPEG}
	_, err := svc.SubmitFace(context.Background(), FaceSubmissionInput{
		ProviderID:    testProviderID,
		CorrelationID: "corr-fail",
		Selfie:        FileUpload{Header: header},
	})
	if err != nil {
		t.Fatalf("SubmitFace() error = %v", err)
	}
	if len(events.faceFailed) != 1 {
		t.Fatalf("faceFailed events = %d, want 1", len(events.faceFailed))
	}
	if events.faceFailed[0].Step != StepFace {
		t.Fatalf("face failed step = %s", events.faceFailed[0].Step)
	}
}

func TestVerificationRejectedEventPublishesOnAdminReject(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	pub := &fakeEventPublisher{}
	svc := newAdminTestService(repo, pub)

	_, err := svc.AdminReview(context.Background(), adminReviewTestAdminID, "corr", adminReviewTestProviderID, AdminReviewRequest{
		Step:   StepIdentity,
		Action: AdminActionReject,
		Reason: "Document is blurry",
	})
	if err != nil {
		t.Fatalf("AdminReview() error = %v", err)
	}
	if len(pub.rejected) != 1 {
		t.Fatalf("rejected events = %d, want 1", len(pub.rejected))
	}
	if pub.rejected[0].Reason != "Document is blurry" {
		t.Fatalf("rejected reason = %s", pub.rejected[0].Reason)
	}
}

func TestVerificationStatusUpdatedDoesNotUseStepLevelStatusAsProviderStatus(t *testing.T) {
	// When publishing status.updated for step-level events, the `VerificationStatus`
	// field must be empty (not "approved"/"rejected") to prevent profile mirror
	// from accidentally setting providers.verification_status to an invalid value.
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	pub := &fakeEventPublisher{}
	svc := newAdminTestService(repo, pub)

	_, err := svc.AdminReview(context.Background(), adminReviewTestAdminID, "corr", adminReviewTestProviderID, AdminReviewRequest{
		Step:   StepIdentity,
		Action: AdminActionApprove,
	})
	if err != nil {
		t.Fatalf("AdminReview() error = %v", err)
	}
	// Find the step-level status.updated event (the one that has Step set but not VerificationStatus)
	var stepLevelFound bool
	for _, ev := range pub.statusUpdated {
		if ev.Step == StepIdentity && ev.VerificationStatus == "" {
			stepLevelFound = true
		}
		// None should have VerificationStatus="approved"
		if ev.VerificationStatus == "approved" {
			t.Fatalf("status.updated has VerificationStatus=approved which is invalid for profile mirror")
		}
	}
	if !stepLevelFound {
		t.Fatalf("no step-level status.updated event found; events = %+v", pub.statusUpdated)
	}
}

func TestVerificationFullyApprovedFiresExactlyOnce(t *testing.T) {
	repo := newFakeVerificationRepository()
	initSteps(repo, adminReviewTestProviderID)
	pub := &fakeEventPublisher{}
	svc := newAdminTestService(repo, pub)
	fullyApproveAllRequired(repo, adminReviewTestProviderID)

	for i := 0; i < 3; i++ {
		_ = svc.markProviderVerifiedIfComplete(context.Background(), adminReviewTestProviderID, "c")
	}
	if len(pub.fullyApproved) != 1 {
		t.Fatalf("fully_approved events = %d, want 1", len(pub.fullyApproved))
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Phase 3L: Security
// ──────────────────────────────────────────────────────────────────────────────

func TestExeUploadRejected(t *testing.T) {
	repo := seededVerificationRepo(t)
	router, tokens := buildVerificationSubmitTestRouter(repo, &captureUploader{}, &fakeFaceMatcher{}, &captureEvents{})

	w := doMultipartProviderRequest(t, router, tokens, http.MethodPost, "/api/v1/provider/verification/identity",
		map[string]string{"govt_id_type": "nin", "govt_id_number": "123"},
		[]testUploadFile{
			{field: "govt_id_file", filename: "evil.exe", contentType: "application/octet-stream", content: []byte("MZ\x90\x00malware")},
			{field: "profile_photo", filename: "me.jpg", contentType: "image/jpeg", content: testJPEG},
		})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for exe; body = %s", w.Code, w.Body.String())
	}
	assertVerificationErrorCode(t, w.Body.Bytes(), "validation_failed")
}

func TestHTMLUploadRejected(t *testing.T) {
	repo := seededVerificationRepo(t)
	router, tokens := buildVerificationSubmitTestRouter(repo, &captureUploader{}, &fakeFaceMatcher{}, &captureEvents{})

	w := doMultipartProviderRequest(t, router, tokens, http.MethodPost, "/api/v1/provider/verification/identity",
		map[string]string{"govt_id_type": "nin", "govt_id_number": "123"},
		[]testUploadFile{
			{field: "govt_id_file", filename: "evil.html", contentType: "text/html", content: []byte("<html><body>xss</body></html>")},
			{field: "profile_photo", filename: "me.jpg", contentType: "image/jpeg", content: testJPEG},
		})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for html; body = %s", w.Code, w.Body.String())
	}
	assertVerificationErrorCode(t, w.Body.Bytes(), "validation_failed")
}

func TestOversizedGovtIDRejected(t *testing.T) {
	repo := seededVerificationRepo(t)
	router, tokens := buildVerificationSubmitTestRouter(repo, &captureUploader{}, &fakeFaceMatcher{}, &captureEvents{})

	big := make([]byte, int(maxGovtIDFileSize)+1)
	copy(big, testJPEG)
	w := doMultipartProviderRequest(t, router, tokens, http.MethodPost, "/api/v1/provider/verification/identity",
		map[string]string{"govt_id_type": "nin", "govt_id_number": "123"},
		[]testUploadFile{
			{field: "govt_id_file", filename: "big.jpg", contentType: "image/jpeg", content: big},
			{field: "profile_photo", filename: "me.jpg", contentType: "image/jpeg", content: testJPEG},
		})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for oversized govt_id; body = %s", w.Code, w.Body.String())
	}
	assertVerificationField(t, w.Body.Bytes(), "govt_id_file")
}

func TestOversizedProfilePhotoRejected(t *testing.T) {
	repo := seededVerificationRepo(t)
	router, tokens := buildVerificationSubmitTestRouter(repo, &captureUploader{}, &fakeFaceMatcher{}, &captureEvents{})

	big := make([]byte, int(maxProfilePhotoSize)+1)
	copy(big, testJPEG)
	w := doMultipartProviderRequest(t, router, tokens, http.MethodPost, "/api/v1/provider/verification/identity",
		map[string]string{"govt_id_type": "nin", "govt_id_number": "123"},
		[]testUploadFile{
			{field: "govt_id_file", filename: "id.pdf", contentType: "application/pdf", content: testPDF},
			{field: "profile_photo", filename: "big.jpg", contentType: "image/jpeg", content: big},
		})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for oversized profile_photo; body = %s", w.Code, w.Body.String())
	}
	assertVerificationField(t, w.Body.Bytes(), "profile_photo")
}

func TestOversizedLicenceFileRejected(t *testing.T) {
	repo := seededVerificationRepo(t)
	router, tokens := buildVerificationSubmitTestRouter(repo, &captureUploader{}, &fakeFaceMatcher{}, &captureEvents{})

	big := make([]byte, int(maxLicenceFileSize)+1)
	copy(big, testJPEG)
	w := doMultipartProviderRequest(t, router, tokens, http.MethodPost, "/api/v1/provider/verification/licence",
		map[string]string{"licence_number": "DRV123", "expiry_year": "2028", "expiry_month": "06"},
		[]testUploadFile{
			{field: "licence_file", filename: "big.jpg", contentType: "image/jpeg", content: big},
		})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for oversized licence; body = %s", w.Code, w.Body.String())
	}
	assertVerificationField(t, w.Body.Bytes(), "licence_file")
}

func TestOversizedSelfieRejected(t *testing.T) {
	repo := seededRepoWithIdentitySubmitted(t)
	router, tokens := buildVerificationSubmitTestRouter(repo, &captureUploader{}, &fakeFaceMatcher{score: 95.0, passed: true}, &captureEvents{})

	big := make([]byte, int(maxSelfieFileSize)+1)
	copy(big, testJPEG)
	w := doMultipartProviderRequest(t, router, tokens, http.MethodPost, "/api/v1/provider/verification/face",
		nil,
		[]testUploadFile{
			{field: "selfie", filename: "big.jpg", contentType: "image/jpeg", content: big},
		})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for oversized selfie; body = %s", w.Code, w.Body.String())
	}
	assertVerificationField(t, w.Body.Bytes(), "selfie")
}

func TestProviderIDFromFormBodyIsIgnored(t *testing.T) {
	// provider_id comes exclusively from JWT. Any provider_id in form fields
	// must be ignored — the file path is keyed only to the JWT identity.
	repo := seededVerificationRepo(t)
	uploader := &captureUploader{}
	router, tokens := buildVerificationSubmitTestRouter(repo, uploader, &fakeFaceMatcher{}, &captureEvents{})

	// Build multipart form that includes a "provider_id" form field with a different value.
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("govt_id_type", "nin")
	_ = w.WriteField("govt_id_number", "12345678901")
	_ = w.WriteField("provider_id", "attacker-uuid-9999-9999-9999") // this must be ignored
	addTestFileToMultipart(t, w, "govt_id_file", "id.pdf", "application/pdf", testPDF)
	addTestFileToMultipart(t, w, "profile_photo", "me.jpg", "image/jpeg", testJPEG)
	w.Close()

	token, _, _ := tokens.GenerateAccessToken(testProviderID, "+2348000000001", "33333333-3333-3333-3333-333333333333")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/verification/identity", &buf)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", w.FormDataContentType())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", rec.Code, rec.Body.String())
	}
	// File path must be under the JWT provider's ID, not the attacker's ID.
	for _, path := range uploader.paths {
		if !strings.HasPrefix(path, "verifications/"+testProviderID+"/") {
			t.Fatalf("upload path %q is not scoped to JWT provider_id=%s", path, testProviderID)
		}
	}
}

func TestStoredURLDoesNotExposeRawFilesystemPath(t *testing.T) {
	repo := seededVerificationRepo(t)
	uploader := &captureUploader{}
	router, tokens := buildVerificationSubmitTestRouter(repo, uploader, &fakeFaceMatcher{}, &captureEvents{})

	w := doMultipartProviderRequest(t, router, tokens, http.MethodPost, "/api/v1/provider/verification/identity",
		map[string]string{"govt_id_type": "nin", "govt_id_number": "12345678901"},
		identityFiles())
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", w.Code, w.Body.String())
	}
	// The captured upload path should not start with an OS root or temp dir.
	for _, path := range uploader.paths {
		if strings.HasPrefix(path, "/") || strings.HasPrefix(path, "C:") || strings.Contains(path, os.TempDir()) {
			t.Fatalf("upload path exposes filesystem: %q", path)
		}
	}
}

func TestUploadValidationHappensBeforeUploaderIsCalled(t *testing.T) {
	repo := seededVerificationRepo(t)
	uploader := &captureUploader{}
	router, tokens := buildVerificationSubmitTestRouter(repo, uploader, &fakeFaceMatcher{}, &captureEvents{})

	// Send an oversized file — validation should reject before any upload.
	big := make([]byte, int(maxGovtIDFileSize)+1)
	copy(big, testJPEG)
	w := doMultipartProviderRequest(t, router, tokens, http.MethodPost, "/api/v1/provider/verification/identity",
		map[string]string{"govt_id_type": "nin", "govt_id_number": "123"},
		[]testUploadFile{
			{field: "govt_id_file", filename: "big.jpg", contentType: "image/jpeg", content: big},
			{field: "profile_photo", filename: "me.jpg", contentType: "image/jpeg", content: testJPEG},
		})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; body = %s", w.Code, w.Body.String())
	}
	if len(uploader.paths) != 0 {
		t.Fatalf("uploader was called despite validation failure: paths = %v", uploader.paths)
	}
}

func TestAdminRouteReturns403ForDispatchProviderJWT(t *testing.T) {
	// Re-verified here for completeness within the 3L test suite.
	router, tokens := buildVerificationTestRouter(newFakeVerificationRepository())
	token, _, _ := tokens.GenerateAccessToken(adminReviewTestAdminID, "+2348000000001", "33333333-3333-3333-3333-333333333333")
	req := httptest.NewRequest(http.MethodPatch,
		"/api/v1/admin/verification/"+adminReviewTestAdminID+"/review",
		strings.NewReader(`{"step":"identity","action":"approve"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
}

func TestNoVehicleUploadEndpointExists(t *testing.T) {
	router, tokens := buildVerificationTestRouter(newFakeVerificationRepository())
	token, _, _ := tokens.GenerateAccessToken(testProviderID, "+2348000000001", "33333333-3333-3333-3333-333333333333")
	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/verification/vehicle", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	// A 404 from Gin means the route does not exist.
	if rec.Code != http.StatusNotFound {
		t.Fatalf("vehicle upload endpoint should not exist: status = %d", rec.Code)
	}
}

func TestPathTraversalFilenameIsSanitized(t *testing.T) {
	// Verify that a malicious filename cannot escape the provider directory.
	path := buildVerificationObjectPath("provider-abc", StepIdentity, "../../etc/passwd")
	if strings.Contains(path, "..") {
		t.Fatalf("objectPath contains path traversal: %q", path)
	}
	if !strings.HasPrefix(path, "verifications/provider-abc/identity/") {
		t.Fatalf("objectPath does not have expected prefix: %q", path)
	}
}

func TestErrorMessageDoesNotExposeStorageInternals(t *testing.T) {
	repo := seededVerificationRepo(t)
	uploader := &captureUploader{err: errStorageFailure}
	router, tokens := buildVerificationSubmitTestRouter(repo, uploader, &fakeFaceMatcher{}, &captureEvents{})

	w := doMultipartProviderRequest(t, router, tokens, http.MethodPost, "/api/v1/provider/verification/identity",
		map[string]string{"govt_id_type": "nin", "govt_id_number": "12345678901"},
		identityFiles())
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d; body = %s", w.Code, w.Body.String())
	}
	lower := strings.ToLower(w.Body.String())
	for _, forbidden := range []string{"s3", "aws", "/tmp", "/var", "c:\\", "bucket", "firebase"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("error message exposes storage internal %q: %s", forbidden, w.Body.String())
		}
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// Additional helpers used by 3L tests
// ──────────────────────────────────────────────────────────────────────────────

var errStorageFailure = errors.New("storage backend unavailable")

func addTestFileToMultipart(t *testing.T, w *multipart.Writer, field, filename, contentType string, content []byte) {
	t.Helper()
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="`+field+`"; filename="`+filename+`"`)
	h.Set("Content-Type", contentType)
	part, err := w.CreatePart(h)
	if err != nil {
		t.Fatalf("create multipart: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write part: %v", err)
	}
}

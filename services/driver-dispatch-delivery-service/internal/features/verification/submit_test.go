package verification

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	authusecases "karrygo/services/driver-dispatch-delivery-service/internal/features/auth/usecases"
	"karrygo/shared/go/apperrors"
	"karrygo/shared/go/httpx"
)

var (
	testJPEG = append([]byte{0xff, 0xd8, 0xff, 0xdb}, bytes.Repeat([]byte{0x00}, 1024)...)
	testPNG  = append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, bytes.Repeat([]byte{0x00}, 1024)...)
	testPDF  = append([]byte("%PDF-1.4\n"), bytes.Repeat([]byte("0"), 1024)...)
)

func TestIdentityValidUploadReturns200AndWritesDocumentsProfilePhotoAndEvent(t *testing.T) {
	repo := seededVerificationRepo(t)
	uploader := &captureUploader{}
	events := &captureEvents{}
	router, tokens := buildVerificationSubmitTestRouter(repo, uploader, &fakeFaceMatcher{}, events)

	w := doMultipartProviderRequest(t, router, tokens, http.MethodPost, "/api/v1/provider/verification/identity", map[string]string{
		"govt_id_type":   "nin",
		"govt_id_number": "12345678901",
	}, []testUploadFile{
		{field: "govt_id_file", filename: "id.pdf", contentType: "application/pdf", content: testPDF},
		{field: "profile_photo", filename: "me.jpg", contentType: "image/jpeg", content: testJPEG},
	})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	step := repo.steps[testProviderID][StepIdentity]
	if step.Status != StatusSubmitted || step.SubmittedAt == nil {
		t.Fatalf("identity step = %+v, want submitted with submitted_at", step)
	}
	if len(repo.documents) != 2 {
		t.Fatalf("documents = %d, want 2: %+v", len(repo.documents), repo.documents)
	}
	assertDocumentType(t, repo.documents, "govt_id")
	assertDocumentType(t, repo.documents, "profile_photo")
	if repo.profilePhotoURLs[testProviderID] == "" {
		t.Fatal("profile_photo_url was not updated")
	}
	if len(uploader.paths) != 2 || !strings.HasPrefix(uploader.paths[0], "verifications/"+testProviderID+"/identity/") || !strings.HasPrefix(uploader.paths[1], "verifications/"+testProviderID+"/identity/") {
		t.Fatalf("upload paths = %+v, want identity paths", uploader.paths)
	}
	if len(events.stepSubmitted) != 1 || events.stepSubmitted[0].Step != StepIdentity {
		t.Fatalf("step submitted events = %+v, want identity event", events.stepSubmitted)
	}
	assertAuditAction(t, repo.auditRows, StepIdentity, AuditActionSubmitted)
}

func TestIdentityValidationFailures(t *testing.T) {
	cases := []struct {
		name      string
		fields    map[string]string
		files     []testUploadFile
		wantField string
	}{
		{
			name:      "missing govt id file",
			fields:    map[string]string{"govt_id_type": "nin", "govt_id_number": "123"},
			files:     []testUploadFile{{field: "profile_photo", filename: "me.jpg", contentType: "image/jpeg", content: testJPEG}},
			wantField: "govt_id_file",
		},
		{
			name:      "missing profile photo",
			fields:    map[string]string{"govt_id_type": "nin", "govt_id_number": "123"},
			files:     []testUploadFile{{field: "govt_id_file", filename: "id.pdf", contentType: "application/pdf", content: testPDF}},
			wantField: "profile_photo",
		},
		{
			name:      "invalid govt id type",
			fields:    map[string]string{"govt_id_type": "school_id", "govt_id_number": "123"},
			files:     identityFiles(),
			wantField: "govt_id_type",
		},
		{
			name:      "oversized govt id",
			fields:    map[string]string{"govt_id_type": "nin", "govt_id_number": "123"},
			files:     []testUploadFile{{field: "govt_id_file", filename: "id.jpg", contentType: "image/jpeg", content: oversizedJPEG(maxGovtIDFileSize)}, {field: "profile_photo", filename: "me.jpg", contentType: "image/jpeg", content: testJPEG}},
			wantField: "govt_id_file",
		},
		{
			name:      "oversized profile photo",
			fields:    map[string]string{"govt_id_type": "nin", "govt_id_number": "123"},
			files:     []testUploadFile{{field: "govt_id_file", filename: "id.pdf", contentType: "application/pdf", content: testPDF}, {field: "profile_photo", filename: "me.jpg", contentType: "image/jpeg", content: oversizedJPEG(maxProfilePhotoSize)}},
			wantField: "profile_photo",
		},
		{
			name:      "unsupported file type",
			fields:    map[string]string{"govt_id_type": "nin", "govt_id_number": "123"},
			files:     []testUploadFile{{field: "govt_id_file", filename: "id.txt", contentType: "text/plain", content: []byte("plain text")}, {field: "profile_photo", filename: "me.jpg", contentType: "image/jpeg", content: testJPEG}},
			wantField: "govt_id_file",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := seededVerificationRepo(t)
			uploader := &captureUploader{}
			router, tokens := buildVerificationSubmitTestRouter(repo, uploader, &fakeFaceMatcher{}, &captureEvents{})

			w := doMultipartProviderRequest(t, router, tokens, http.MethodPost, "/api/v1/provider/verification/identity", tc.fields, tc.files)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
			}
			assertVerificationErrorCode(t, w.Body.Bytes(), apperrors.CodeValidationFailed)
			assertVerificationField(t, w.Body.Bytes(), tc.wantField)
			if len(uploader.paths) != 0 {
				t.Fatalf("uploaded files despite validation failure: %+v", uploader.paths)
			}
		})
	}
}

func TestIdentityStorageFailureUsesGenericWording(t *testing.T) {
	repo := seededVerificationRepo(t)
	uploader := &captureUploader{err: errors.New("disk full")}
	router, tokens := buildVerificationSubmitTestRouter(repo, uploader, &fakeFaceMatcher{}, &captureEvents{})

	w := doMultipartProviderRequest(t, router, tokens, http.MethodPost, "/api/v1/provider/verification/identity", map[string]string{
		"govt_id_type":   "nin",
		"govt_id_number": "12345678901",
	}, identityFiles())

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body = %s", w.Code, w.Body.String())
	}
	body := strings.ToLower(w.Body.String())
	if !strings.Contains(body, "storage upload failed") {
		t.Fatalf("body = %s, want generic storage upload failure", w.Body.String())
	}
	if strings.Contains(strings.ToLower(w.Body.String()), "s"+"3") {
		t.Fatalf("body = %s, want no provider-specific wording", w.Body.String())
	}
}

func TestIdentityAlreadyApprovedReturns409AndMissingStepReturns412(t *testing.T) {
	t.Run("approved", func(t *testing.T) {
		repo := seededVerificationRepo(t)
		step := repo.steps[testProviderID][StepIdentity]
		step.Status = StatusApproved
		repo.steps[testProviderID][StepIdentity] = step
		router, tokens := buildVerificationSubmitTestRouter(repo, &captureUploader{}, &fakeFaceMatcher{}, &captureEvents{})

		w := doMultipartProviderRequest(t, router, tokens, http.MethodPost, "/api/v1/provider/verification/identity", map[string]string{"govt_id_type": "nin", "govt_id_number": "123"}, identityFiles())
		if w.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409; body = %s", w.Code, w.Body.String())
		}
		assertVerificationErrorCode(t, w.Body.Bytes(), apperrors.CodeConflict)
	})
	t.Run("missing step", func(t *testing.T) {
		repo := newFakeVerificationRepository()
		router, tokens := buildVerificationSubmitTestRouter(repo, &captureUploader{}, &fakeFaceMatcher{}, &captureEvents{})

		w := doMultipartProviderRequest(t, router, tokens, http.MethodPost, "/api/v1/provider/verification/identity", map[string]string{"govt_id_type": "nin", "govt_id_number": "123"}, identityFiles())
		if w.Code != http.StatusPreconditionFailed {
			t.Fatalf("status = %d, want 412; body = %s", w.Code, w.Body.String())
		}
		assertVerificationErrorCode(t, w.Body.Bytes(), apperrors.Code("precondition_failed"))
	})
}

func TestIdentityResubmitAfterRejectResetsStatusAndInsertsResubmittedAudit(t *testing.T) {
	repo := seededVerificationRepo(t)
	// Simulate a previously rejected identity step.
	reason := "Document expired."
	step := repo.steps[testProviderID][StepIdentity]
	step.Status = StatusRejected
	step.RejectionReason = &reason
	repo.steps[testProviderID][StepIdentity] = step

	events := &captureEvents{}
	router, tokens := buildVerificationSubmitTestRouter(repo, &captureUploader{}, &fakeFaceMatcher{}, events)

	w := doMultipartProviderRequest(t, router, tokens, http.MethodPost, "/api/v1/provider/verification/identity",
		map[string]string{"govt_id_type": "nin", "govt_id_number": "12345678901"},
		identityFiles())
	if w.Code != http.StatusOK {
		t.Fatalf("resubmit status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	updated := repo.steps[testProviderID][StepIdentity]
	if updated.Status != StatusSubmitted {
		t.Fatalf("identity status = %s, want submitted after resubmit", updated.Status)
	}
	if updated.RejectionReason != nil {
		t.Fatalf("rejection_reason = %v, want nil after resubmit", updated.RejectionReason)
	}
	assertAuditAction(t, repo.auditRows, StepIdentity, AuditActionResubmitted)
	if len(events.stepSubmitted) != 1 || events.stepSubmitted[0].Step != StepIdentity {
		t.Fatalf("step submitted event missing after resubmit: %+v", events.stepSubmitted)
	}
}

func TestLicenceValidUploadReturns200WritesDocumentAndPublishesEvent(t *testing.T) {
	repo := seededVerificationRepo(t)
	uploader := &captureUploader{}
	events := &captureEvents{}
	router, tokens := buildVerificationSubmitTestRouter(repo, uploader, &fakeFaceMatcher{}, events)

	w := doMultipartProviderRequest(t, router, tokens, http.MethodPost, "/api/v1/provider/verification/licence", map[string]string{
		"licence_number": "DRV123",
		"expiry_year":    "2028",
		"expiry_month":   "09",
	}, []testUploadFile{{field: "licence_file", filename: "licence.pdf", contentType: "application/pdf", content: testPDF}})

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	if repo.steps[testProviderID][StepLicence].Status != StatusSubmitted {
		t.Fatalf("licence step = %+v, want submitted", repo.steps[testProviderID][StepLicence])
	}
	assertDocumentType(t, repo.documents, "licence_doc")
	if len(uploader.paths) != 1 || !strings.HasPrefix(uploader.paths[0], "verifications/"+testProviderID+"/licence/") {
		t.Fatalf("upload paths = %+v, want licence path", uploader.paths)
	}
	if len(events.stepSubmitted) != 1 || events.stepSubmitted[0].Step != StepLicence {
		t.Fatalf("events = %+v, want licence submitted", events.stepSubmitted)
	}
}

func TestLicenceValidationConflictAndPrecondition(t *testing.T) {
	cases := []struct {
		name      string
		fields    map[string]string
		files     []testUploadFile
		wantField string
	}{
		{name: "invalid month", fields: map[string]string{"licence_number": "DRV123", "expiry_year": "2028", "expiry_month": "13"}, files: licenceFiles(), wantField: "expiry_month"},
		{name: "invalid year", fields: map[string]string{"licence_number": "DRV123", "expiry_year": "28", "expiry_month": "09"}, files: licenceFiles(), wantField: "expiry_year"},
		{name: "missing file", fields: map[string]string{"licence_number": "DRV123", "expiry_year": "2028", "expiry_month": "09"}, files: nil, wantField: "licence_file"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := seededVerificationRepo(t)
			router, tokens := buildVerificationSubmitTestRouter(repo, &captureUploader{}, &fakeFaceMatcher{}, &captureEvents{})
			w := doMultipartProviderRequest(t, router, tokens, http.MethodPost, "/api/v1/provider/verification/licence", tc.fields, tc.files)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
			}
			assertVerificationField(t, w.Body.Bytes(), tc.wantField)
		})
	}

	t.Run("approved", func(t *testing.T) {
		repo := seededVerificationRepo(t)
		step := repo.steps[testProviderID][StepLicence]
		step.Status = StatusApproved
		repo.steps[testProviderID][StepLicence] = step
		router, tokens := buildVerificationSubmitTestRouter(repo, &captureUploader{}, &fakeFaceMatcher{}, &captureEvents{})
		w := doMultipartProviderRequest(t, router, tokens, http.MethodPost, "/api/v1/provider/verification/licence", licenceFields(), licenceFiles())
		if w.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409; body = %s", w.Code, w.Body.String())
		}
	})

	t.Run("missing step", func(t *testing.T) {
		repo := newFakeVerificationRepository()
		router, tokens := buildVerificationSubmitTestRouter(repo, &captureUploader{}, &fakeFaceMatcher{}, &captureEvents{})
		w := doMultipartProviderRequest(t, router, tokens, http.MethodPost, "/api/v1/provider/verification/licence", licenceFields(), licenceFiles())
		if w.Code != http.StatusPreconditionFailed {
			t.Fatalf("status = %d, want 412; body = %s", w.Code, w.Body.String())
		}
	})

	t.Run("optional skip gate", func(t *testing.T) {
		repo := seededVerificationRepo(t)
		licence := repo.steps[testProviderID][StepLicence]
		if !licence.IsOptional || licence.Status != StatusPending {
			t.Fatalf("licence = %+v, want optional pending step that may be skipped", licence)
		}
	})
}

func TestFaceHighScorePassSubmitsStepCreatesCheckAndPublishesEvent(t *testing.T) {
	repo := seededFaceReadyRepo(t)
	events := &captureEvents{}
	router, tokens := buildVerificationSubmitTestRouter(repo, &captureUploader{}, &fakeFaceMatcher{score: 92.4, passed: true}, events)

	w := doMultipartProviderRequest(t, router, tokens, http.MethodPost, "/api/v1/provider/verification/face", nil, selfieFiles(testJPEG, "image/jpeg"))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	if repo.steps[testProviderID][StepFace].Status != StatusSubmitted {
		t.Fatalf("face step = %+v, want submitted", repo.steps[testProviderID][StepFace])
	}
	if len(repo.faceChecks) != 1 || repo.faceChecks[0].Result == nil || *repo.faceChecks[0].Result != "pass" {
		t.Fatalf("face checks = %+v, want pass row", repo.faceChecks)
	}
	if len(events.stepSubmitted) != 1 || events.stepSubmitted[0].Step != StepFace {
		t.Fatalf("events = %+v, want face submitted", events.stepSubmitted)
	}
}

func TestFaceLowScoreReturnsFailLeavesPendingAndCanRetry(t *testing.T) {
	repo := seededFaceReadyRepo(t)
	events := &captureEvents{}
	router, tokens := buildVerificationSubmitTestRouter(repo, &captureUploader{}, &fakeFaceMatcher{score: 34.1, passed: false}, events)

	for i := 0; i < 2; i++ {
		w := doMultipartProviderRequest(t, router, tokens, http.MethodPost, "/api/v1/provider/verification/face", nil, selfieFiles(testPNG, "image/png"))
		if w.Code != http.StatusOK {
			t.Fatalf("attempt %d status = %d, want 200; body = %s", i+1, w.Code, w.Body.String())
		}
	}
	if repo.steps[testProviderID][StepFace].Status != StatusPending {
		t.Fatalf("face step = %+v, want pending after fail", repo.steps[testProviderID][StepFace])
	}
	if len(repo.faceChecks) != 2 {
		t.Fatalf("face checks = %d, want 2 retry rows", len(repo.faceChecks))
	}
	if len(events.faceFailed) != 2 {
		t.Fatalf("face failed events = %d, want 2", len(events.faceFailed))
	}
	assertAuditAction(t, repo.auditRows, StepFace, AuditActionFaceFailed)
}

func TestFaceValidationPreconditionConflictAndProviderErrors(t *testing.T) {
	t.Run("identity required", func(t *testing.T) {
		repo := seededVerificationRepo(t)
		router, tokens := buildVerificationSubmitTestRouter(repo, &captureUploader{}, &fakeFaceMatcher{score: 92, passed: true}, &captureEvents{})
		w := doMultipartProviderRequest(t, router, tokens, http.MethodPost, "/api/v1/provider/verification/face", nil, selfieFiles(testJPEG, "image/jpeg"))
		if w.Code != http.StatusPreconditionFailed {
			t.Fatalf("status = %d, want 412; body = %s", w.Code, w.Body.String())
		}
	})
	t.Run("govt id doc required", func(t *testing.T) {
		repo := seededVerificationRepo(t)
		step := repo.steps[testProviderID][StepIdentity]
		step.Status = StatusSubmitted
		repo.steps[testProviderID][StepIdentity] = step
		router, tokens := buildVerificationSubmitTestRouter(repo, &captureUploader{}, &fakeFaceMatcher{score: 92, passed: true}, &captureEvents{})
		w := doMultipartProviderRequest(t, router, tokens, http.MethodPost, "/api/v1/provider/verification/face", nil, selfieFiles(testJPEG, "image/jpeg"))
		if w.Code != http.StatusPreconditionFailed {
			t.Fatalf("status = %d, want 412; body = %s", w.Code, w.Body.String())
		}
	})
	t.Run("missing selfie", func(t *testing.T) {
		repo := seededFaceReadyRepo(t)
		router, tokens := buildVerificationSubmitTestRouter(repo, &captureUploader{}, &fakeFaceMatcher{score: 92, passed: true}, &captureEvents{})
		w := doMultipartProviderRequest(t, router, tokens, http.MethodPost, "/api/v1/provider/verification/face", nil, nil)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
		}
		assertVerificationField(t, w.Body.Bytes(), "selfie")
	})
	t.Run("unsupported selfie", func(t *testing.T) {
		repo := seededFaceReadyRepo(t)
		router, tokens := buildVerificationSubmitTestRouter(repo, &captureUploader{}, &fakeFaceMatcher{score: 92, passed: true}, &captureEvents{})
		w := doMultipartProviderRequest(t, router, tokens, http.MethodPost, "/api/v1/provider/verification/face", nil, []testUploadFile{{field: "selfie", filename: "selfie.txt", contentType: "text/plain", content: []byte("nope")}})
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
		}
	})
	t.Run("oversized selfie", func(t *testing.T) {
		repo := seededFaceReadyRepo(t)
		router, tokens := buildVerificationSubmitTestRouter(repo, &captureUploader{}, &fakeFaceMatcher{score: 92, passed: true}, &captureEvents{})
		w := doMultipartProviderRequest(t, router, tokens, http.MethodPost, "/api/v1/provider/verification/face", nil, selfieFiles(oversizedJPEG(maxSelfieFileSize), "image/jpeg"))
		if w.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400; body = %s", w.Code, w.Body.String())
		}
	})
	t.Run("approved face", func(t *testing.T) {
		repo := seededFaceReadyRepo(t)
		step := repo.steps[testProviderID][StepFace]
		step.Status = StatusApproved
		repo.steps[testProviderID][StepFace] = step
		router, tokens := buildVerificationSubmitTestRouter(repo, &captureUploader{}, &fakeFaceMatcher{score: 92, passed: true}, &captureEvents{})
		w := doMultipartProviderRequest(t, router, tokens, http.MethodPost, "/api/v1/provider/verification/face", nil, selfieFiles(testJPEG, "image/jpeg"))
		if w.Code != http.StatusConflict {
			t.Fatalf("status = %d, want 409; body = %s", w.Code, w.Body.String())
		}
	})
	t.Run("smile unavailable", func(t *testing.T) {
		repo := seededFaceReadyRepo(t)
		router, tokens := buildVerificationSubmitTestRouter(repo, &captureUploader{}, &fakeFaceMatcher{err: errors.New("smile down")}, &captureEvents{})
		w := doMultipartProviderRequest(t, router, tokens, http.MethodPost, "/api/v1/provider/verification/face", nil, selfieFiles(testJPEG, "image/jpeg"))
		if w.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500; body = %s", w.Code, w.Body.String())
		}
		if len(repo.faceChecks) != 1 || repo.faceChecks[0].ErrorMessage == nil {
			t.Fatalf("face check error was not stored: %+v", repo.faceChecks)
		}
	})
}

type testUploadFile struct {
	field       string
	filename    string
	contentType string
	content     []byte
}

func buildVerificationSubmitTestRouter(repo Repository, uploader FileUploader, face FaceMatcher, events EventPublisher) (*gin.Engine, *authusecases.TokenUsecase) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpx.RequestID())
	router.Use(httpx.Recovery())
	router.Use(httpx.ErrorHandler())

	tokens := authusecases.NewTokenUsecase([]byte("verification-test-secret"), 15*timeMinute(), 30*24*timeHour())
	handler := NewHandlerWithService(NewService(repo, face, WithUploader(uploader), WithEventPublisher(events)))
	RegisterRoutes(router, tokens, handler)
	return router, tokens
}

func doMultipartProviderRequest(t *testing.T, router *gin.Engine, tokens *authusecases.TokenUsecase, method string, path string, fields map[string]string, files []testUploadFile) *httptest.ResponseRecorder {
	t.Helper()
	body, contentType := buildMultipartBody(t, fields, files)
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", contentType)
	token, _, err := tokens.GenerateAccessToken(testProviderID, "+2348000000001", "session-123")
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func buildMultipartBody(t *testing.T, fields map[string]string, files []testUploadFile) (*bytes.Buffer, string) {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatalf("write field %s: %v", key, err)
		}
	}
	for _, file := range files {
		header := make(textproto.MIMEHeader)
		header.Set("Content-Disposition", `form-data; name="`+file.field+`"; filename="`+file.filename+`"`)
		header.Set("Content-Type", file.contentType)
		part, err := writer.CreatePart(header)
		if err != nil {
			t.Fatalf("create file part: %v", err)
		}
		if _, err := part.Write(file.content); err != nil {
			t.Fatalf("write file part: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return body, writer.FormDataContentType()
}

func seededVerificationRepo(t *testing.T) *fakeVerificationRepository {
	t.Helper()
	repo := newFakeVerificationRepository()
	if _, err := repo.InitializeStepsForProvider(context.Background(), testProviderID); err != nil {
		t.Fatalf("InitializeStepsForProvider() error = %v", err)
	}
	return repo
}

func seededFaceReadyRepo(t *testing.T) *fakeVerificationRepository {
	t.Helper()
	repo := seededVerificationRepo(t)
	step := repo.steps[testProviderID][StepIdentity]
	step.Status = StatusSubmitted
	repo.steps[testProviderID][StepIdentity] = step
	size := len(testPDF)
	mimeType := "application/pdf"
	repo.documents = append(repo.documents, VerificationDocument{
		ID:           "doc-govt-id",
		StepID:       step.ID,
		ProviderID:   testProviderID,
		DocumentType: "govt_id",
		FileURL:      "local-private://verifications/" + testProviderID + "/identity/id.pdf",
		FileSize:     &size,
		MimeType:     &mimeType,
	})
	return repo
}

func identityFiles() []testUploadFile {
	return []testUploadFile{
		{field: "govt_id_file", filename: "id.pdf", contentType: "application/pdf", content: testPDF},
		{field: "profile_photo", filename: "me.jpg", contentType: "image/jpeg", content: testJPEG},
	}
}

func licenceFields() map[string]string {
	return map[string]string{"licence_number": "DRV123", "expiry_year": "2028", "expiry_month": "09"}
}

func licenceFiles() []testUploadFile {
	return []testUploadFile{{field: "licence_file", filename: "licence.pdf", contentType: "application/pdf", content: testPDF}}
}

func selfieFiles(content []byte, contentType string) []testUploadFile {
	return []testUploadFile{{field: "selfie", filename: "selfie.jpg", contentType: contentType, content: content}}
}

func oversizedJPEG(limit int64) []byte {
	payload := make([]byte, int(limit)+1)
	copy(payload, testJPEG[:4])
	return payload
}

func assertDocumentType(t *testing.T, docs []VerificationDocument, documentType string) {
	t.Helper()
	for _, doc := range docs {
		if doc.DocumentType == documentType {
			return
		}
	}
	t.Fatalf("missing document type %s in %+v", documentType, docs)
}

func assertAuditAction(t *testing.T, audits []VerificationAudit, step Step, action AuditAction) {
	t.Helper()
	for _, audit := range audits {
		if audit.Step == step && audit.Action == action {
			return
		}
	}
	t.Fatalf("missing audit step=%s action=%s in %+v", step, action, audits)
}

func assertVerificationField(t *testing.T, raw []byte, field string) {
	t.Helper()
	if !bytes.Contains(raw, []byte(`"field":"`+field+`"`)) {
		t.Fatalf("field %s not found in body %s", field, raw)
	}
}

type captureUploader struct {
	paths []string
	err   error
}

func (u *captureUploader) Upload(ctx context.Context, path string, file File, header FileHeader) (string, error) {
	if u.err != nil {
		return "", u.err
	}
	_, _ = io.Copy(io.Discard, file)
	u.paths = append(u.paths, path)
	return "local-private://" + path, nil
}

type fakeFaceMatcher struct {
	score  float64
	passed bool
	err    error
}

func (f *fakeFaceMatcher) MatchFace(ctx context.Context, selfieURL string, idDocURL string) (FaceMatchResult, error) {
	if f.err != nil {
		return FaceMatchResult{}, f.err
	}
	return FaceMatchResult{MatchScore: f.score, Passed: f.passed, RawResponse: "{}"}, nil
}

type captureEvents struct {
	stepSubmitted []StepSubmittedEvent
	faceFailed    []FaceFailedEvent
	err           error
}

func (e *captureEvents) PublishStepSubmitted(ctx context.Context, event StepSubmittedEvent) error {
	if e.err != nil {
		return e.err
	}
	e.stepSubmitted = append(e.stepSubmitted, event)
	return nil
}

func (e *captureEvents) PublishVerificationStatusUpdated(_ context.Context, _ VerificationStatusUpdatedEvent) error {
	return e.err
}

func (e *captureEvents) PublishVerificationFullyApproved(_ context.Context, _ VerificationFullyApprovedEvent) error {
	return e.err
}

func (e *captureEvents) PublishVerificationRejected(_ context.Context, _ VerificationRejectedEvent) error {
	return e.err
}

func (e *captureEvents) PublishFaceFailed(ctx context.Context, event FaceFailedEvent) error {
	if e.err != nil {
		return e.err
	}
	e.faceFailed = append(e.faceFailed, event)
	return nil
}

func timeMinute() time.Duration {
	return time.Minute
}

func timeHour() time.Duration {
	return time.Hour
}

package verification

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"cosmicforge/logistics/shared/go/apperrors"
	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
)

func TestGetAllStatusReturnsAllSixSteps(t *testing.T) {
	repo := seededVerificationRepo(t)
	router, tokens := buildVerificationTestRouter(repo)

	w := doProviderGet(t, router, tokens, "/api/v1/provider/verification/status")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}

	data := decodeStatusResponse(t, w.Body.Bytes())
	if len(data.Steps) != 6 {
		t.Fatalf("steps = %d, want 6: %+v", len(data.Steps), data.Steps)
	}
	if data.OverallStatus != OverallStatusNotStarted {
		t.Fatalf("overall_status = %s, want not_started", data.OverallStatus)
	}
	if data.CompletionPercentage != 40 {
		t.Fatalf("completion_percentage = %d, want 40", data.CompletionPercentage)
	}
	if data.Steps[1].Step != StepLicence || !data.Steps[1].IsOptional {
		t.Fatalf("licence step = %+v, want optional licence in second position", data.Steps[1])
	}
}

func TestGetAllStatusReturns404WhenStepsDoNotExist(t *testing.T) {
	router, tokens := buildVerificationTestRouter(newFakeVerificationRepository())

	w := doProviderGet(t, router, tokens, "/api/v1/provider/verification/status")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
	assertVerificationErrorCode(t, w.Body.Bytes(), apperrors.CodeNotFound)
}

func TestCompletionPercentageExcludesLicence(t *testing.T) {
	repo := seededVerificationRepo(t)
	setStepStatus(t, repo, StepLicence, StatusApproved)
	router, tokens := buildVerificationTestRouter(repo)

	w := doProviderGet(t, router, tokens, "/api/v1/provider/verification/status")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	data := decodeStatusResponse(t, w.Body.Bytes())
	if data.CompletionPercentage != 40 {
		t.Fatalf("completion_percentage = %d, want 40 because licence is optional", data.CompletionPercentage)
	}
}

func TestCompletionPercentageIs100WhenRequiredStepsApprovedAndLicencePending(t *testing.T) {
	repo := seededVerificationRepo(t)
	approveRequiredSteps(t, repo)
	router, tokens := buildVerificationTestRouter(repo)

	w := doProviderGet(t, router, tokens, "/api/v1/provider/verification/status")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	data := decodeStatusResponse(t, w.Body.Bytes())
	if data.CompletionPercentage != 100 {
		t.Fatalf("completion_percentage = %d, want 100", data.CompletionPercentage)
	}
	if data.OverallStatus != OverallStatusVerified {
		t.Fatalf("overall_status = %s, want verified", data.OverallStatus)
	}
	if repo.steps[testProviderID][StepLicence].Status != StatusPending {
		t.Fatalf("licence status = %s, want still pending", repo.steps[testProviderID][StepLicence].Status)
	}
}

func TestOverallStatusRules(t *testing.T) {
	cases := []struct {
		name    string
		setup   func(*testing.T, *fakeVerificationRepository)
		want    OverallStatus
		wantPct int
	}{
		{
			name:    "not started with only auto approved steps",
			setup:   func(t *testing.T, repo *fakeVerificationRepository) {},
			want:    OverallStatusNotStarted,
			wantPct: 40,
		},
		{
			name: "in progress when one manual required step is submitted",
			setup: func(t *testing.T, repo *fakeVerificationRepository) {
				setStepStatus(t, repo, StepIdentity, StatusSubmitted)
			},
			want:    OverallStatusInProgress,
			wantPct: 40,
		},
		{
			name: "pending review when all required steps are submitted or approved",
			setup: func(t *testing.T, repo *fakeVerificationRepository) {
				setStepStatus(t, repo, StepIdentity, StatusSubmitted)
				setStepStatus(t, repo, StepVehicle, StatusApproved)
				setStepStatus(t, repo, StepFace, StatusSubmitted)
			},
			want:    OverallStatusPendingReview,
			wantPct: 60,
		},
		{
			name: "verified when all required steps are approved",
			setup: func(t *testing.T, repo *fakeVerificationRepository) {
				approveRequiredSteps(t, repo)
			},
			want:    OverallStatusVerified,
			wantPct: 100,
		},
		{
			name: "rejected when any required step is rejected",
			setup: func(t *testing.T, repo *fakeVerificationRepository) {
				setStepStatus(t, repo, StepIdentity, StatusRejected)
			},
			want:    OverallStatusRejected,
			wantPct: 40,
		},
		{
			name: "suspended when provider state is suspended",
			setup: func(t *testing.T, repo *fakeVerificationRepository) {
				repo.providerStates[testProviderID] = ProviderVerificationState{ProviderID: testProviderID, VerificationStatus: "suspended", IsActive: true}
			},
			want:    OverallStatusSuspended,
			wantPct: 40,
		},
		{
			name: "suspended when provider is inactive",
			setup: func(t *testing.T, repo *fakeVerificationRepository) {
				repo.providerStates[testProviderID] = ProviderVerificationState{ProviderID: testProviderID, VerificationStatus: "unverified", IsActive: false}
			},
			want:    OverallStatusSuspended,
			wantPct: 40,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := seededVerificationRepo(t)
			tc.setup(t, repo)

			result, err := NewService(repo, NewStubSmileIdentityClient()).GetAllStatus(t.Context(), testProviderID)
			if err != nil {
				t.Fatalf("GetAllStatus() error = %v", err)
			}
			if result.OverallStatus != tc.want {
				t.Fatalf("overall_status = %s, want %s", result.OverallStatus, tc.want)
			}
			if result.CompletionPercentage != tc.wantPct {
				t.Fatalf("completion_percentage = %d, want %d", result.CompletionPercentage, tc.wantPct)
			}
		})
	}
}

func TestGetStepStatusReturnsIdentityDetailWithDocuments(t *testing.T) {
	repo := seededVerificationRepo(t)
	step := repo.steps[testProviderID][StepIdentity]
	size := 123
	mimeType := "application/pdf"
	repo.documents = append(repo.documents, VerificationDocument{
		ID:           "doc-identity",
		StepID:       step.ID,
		ProviderID:   testProviderID,
		DocumentType: "govt_id",
		FileURL:      "local-private://verifications/" + testProviderID + "/identity/id.pdf",
		FileSize:     &size,
		MimeType:     &mimeType,
		UploadedAt:   time.Now().UTC(),
	})
	router, tokens := buildVerificationTestRouter(repo)

	w := doProviderGet(t, router, tokens, "/api/v1/provider/verification/status/identity")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	data := decodeStepStatusResponse(t, w.Body.Bytes())
	if data.Step != StepIdentity || len(data.Documents) != 1 {
		t.Fatalf("identity response = %+v, want one document", data)
	}
	if data.Documents[0].FileURL != "local-private://verifications/"+testProviderID+"/identity/id.pdf" {
		t.Fatalf("file_url = %q, want stored private reference", data.Documents[0].FileURL)
	}
}

func TestGetStepStatusReturnsLicenceDetailWithDocuments(t *testing.T) {
	repo := seededVerificationRepo(t)
	step := repo.steps[testProviderID][StepLicence]
	repo.documents = append(repo.documents, VerificationDocument{
		ID:           "doc-licence",
		StepID:       step.ID,
		ProviderID:   testProviderID,
		DocumentType: "licence_doc",
		FileURL:      "local-private://verifications/" + testProviderID + "/licence/licence.pdf",
		UploadedAt:   time.Now().UTC(),
	})
	router, tokens := buildVerificationTestRouter(repo)

	w := doProviderGet(t, router, tokens, "/api/v1/provider/verification/status/licence")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	data := decodeStepStatusResponse(t, w.Body.Bytes())
	if data.Step != StepLicence || !data.IsOptional || len(data.Documents) != 1 {
		t.Fatalf("licence response = %+v, want optional step with one document", data)
	}
}

func TestGetStepStatusReturnsFaceDetailWithLastFaceCheck(t *testing.T) {
	repo := seededVerificationRepo(t)
	setStepStatus(t, repo, StepFace, StatusSubmitted)
	result := "pass"
	score := 92.4
	checkedAt := time.Now().UTC()
	repo.faceChecks = append(repo.faceChecks, FaceCheck{
		ID:         "face-check-1",
		ProviderID: testProviderID,
		StepID:     repo.steps[testProviderID][StepFace].ID,
		Result:     &result,
		MatchScore: &score,
		CheckedAt:  &checkedAt,
	})
	router, tokens := buildVerificationTestRouter(repo)

	w := doProviderGet(t, router, tokens, "/api/v1/provider/verification/status/face")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	data := decodeStepStatusResponse(t, w.Body.Bytes())
	if data.LastFaceCheck == nil || data.LastFaceCheck.Result == nil || *data.LastFaceCheck.Result != "pass" {
		t.Fatalf("last_face_check = %+v, want pass detail", data.LastFaceCheck)
	}
	if data.LastFaceCheck.MatchScore == nil || *data.LastFaceCheck.MatchScore != score {
		t.Fatalf("match_score = %+v, want %v", data.LastFaceCheck.MatchScore, score)
	}
}

func TestGetStepStatusReturns404ForUnknownStep(t *testing.T) {
	repo := seededVerificationRepo(t)
	router, tokens := buildVerificationTestRouter(repo)

	w := doProviderGet(t, router, tokens, "/api/v1/provider/verification/status/not-a-step")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
	assertVerificationErrorCode(t, w.Body.Bytes(), apperrors.CodeNotFound)
}

func TestGetStepStatusReturns404WhenStepRowMissing(t *testing.T) {
	repo := newFakeVerificationRepository()
	router, tokens := buildVerificationTestRouter(repo)

	w := doProviderGet(t, router, tokens, "/api/v1/provider/verification/status/identity")
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", w.Code, w.Body.String())
	}
	assertVerificationErrorCode(t, w.Body.Bytes(), apperrors.CodeNotFound)
}

func TestProviderVerificationVehicleRouteIsNotRegistered(t *testing.T) {
	repo := seededVerificationRepo(t)
	router, tokens := buildVerificationTestRouter(repo)
	token, _, err := tokens.GenerateAccessToken(testProviderID, "+2348000000001", "33333333-3333-3333-3333-333333333333")
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/provider/verification"+"/vehicle", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 for absent vehicle upload route; body = %s", w.Code, w.Body.String())
	}
}

func doProviderGet(t *testing.T, router http.Handler, tokens *authusecases.TokenUsecase, path string) *httptest.ResponseRecorder {
	t.Helper()
	token, _, err := tokens.GenerateAccessToken(testProviderID, "+2348000000001", "33333333-3333-3333-3333-333333333333")
	if err != nil {
		t.Fatalf("GenerateAccessToken() error = %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func decodeStatusResponse(t *testing.T, raw []byte) AllStatusResponse {
	t.Helper()
	var resp struct {
		Success bool              `json:"success"`
		Data    AllStatusResponse `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal status response: %v; body = %s", err, raw)
	}
	if !resp.Success {
		t.Fatalf("success = false; body = %s", raw)
	}
	return resp.Data
}

func decodeStepStatusResponse(t *testing.T, raw []byte) StepStatusResponse {
	t.Helper()
	var resp struct {
		Success bool               `json:"success"`
		Data    StepStatusResponse `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal step response: %v; body = %s", err, raw)
	}
	if !resp.Success {
		t.Fatalf("success = false; body = %s", raw)
	}
	return resp.Data
}

func approveRequiredSteps(t *testing.T, repo *fakeVerificationRepository) {
	t.Helper()
	for _, step := range requiredVerificationSteps() {
		setStepStatus(t, repo, step, StatusApproved)
	}
}

func setStepStatus(t *testing.T, repo *fakeVerificationRepository, step Step, status StepStatus) {
	t.Helper()
	value, ok := repo.steps[testProviderID][step]
	if !ok {
		t.Fatalf("step %s missing", step)
	}
	now := time.Now().UTC()
	value.Status = status
	switch status {
	case StatusSubmitted:
		value.SubmittedAt = &now
	case StatusApproved:
		value.ReviewedAt = &now
	case StatusRejected:
		reason := "Test rejection."
		value.RejectionReason = &reason
		value.ReviewedAt = &now
	}
	value.UpdatedAt = now
	repo.steps[testProviderID][step] = value
}

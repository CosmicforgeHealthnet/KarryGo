package verification

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	authusecases "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/usecases"
	"cosmicforge/logistics/shared/go/httpx"
)

// This is deliberately NOT an automated face match or liveness test.
type forbiddenExternalMatcher struct{ called bool }

func (m *forbiddenExternalMatcher) MatchFace(context.Context, string, string) (FaceMatchResult, error) {
	m.called = true
	return FaceMatchResult{Passed: true, MatchScore: 100}, nil
}

func newInHouseRouter(repo Repository, matcher FaceMatcher) (*gin.Engine, *authusecases.TokenUsecase, *captureEvents) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpx.RequestID(), httpx.Recovery(), httpx.ErrorHandler())
	tokens := authusecases.NewTokenUsecase([]byte("manual-review-test-secret"), 15*timeMinute(), 30*24*timeHour())
	events := &captureEvents{}
	handler := NewHandlerWithService(NewService(repo, matcher,
		WithInHouseManualFaceReview(),
		WithUploader(&captureUploader{}),
		WithEventPublisher(events),
	))
	RegisterRoutes(router, tokens, handler)
	return router, tokens, events
}

// A real decodable JPEG is needed by the mechanical quality prescreen.
func testGradientSelfie(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 320, 320))
	for y := 0; y < 320; y++ {
		for x := 0; x < 320; x++ {
			level := uint8(60 + ((x*7 + y*11 + x*y/80) % 135))
			img.Set(x, y, color.RGBA{R: level, G: level, B: level, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil { t.Fatal(err) }
	return buf.Bytes()
}

func submitForReview(t *testing.T, repo *fakeVerificationRepository) (*gin.Engine, *authusecases.TokenUsecase, *forbiddenExternalMatcher) {
	t.Helper()
	matcher := &forbiddenExternalMatcher{}
	router, tokens, _ := newInHouseRouter(repo, matcher)
	w := doMultipartProviderRequest(t, router, tokens, http.MethodPost,
		"/api/v1/provider/verification/face", nil, selfieFiles(testGradientSelfie(t), "image/jpeg"))
	if w.Code != http.StatusOK {
		t.Fatalf("submit face status=%d body=%s", w.Code, w.Body.String())
	}
	var body struct { Data FaceSubmissionResponse `json:"data"` }
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil { t.Fatal(err) }
	if body.Data.Result != "pending_review" || body.Data.Status != StatusSubmitted {
		t.Fatalf("submission unexpectedly approved: %+v", body.Data)
	}
	if matcher.called { t.Fatal("in-house review called an external matcher") }
	if repo.steps[testProviderID][StepFace].Status != StatusSubmitted {
		t.Fatal("face must remain submitted until a real admin approves")
	}
	if len(repo.faceChecks) != 1 ||
		repo.faceChecks[0].ProviderUsed != "karryam_manual_review" ||
		repo.faceChecks[0].MatchScore != nil ||
		repo.faceChecks[0].Result == nil ||
		*repo.faceChecks[0].Result != "pending_review" {
		t.Fatalf("in-house evidence row must have no synthetic score: %+v", repo.faceChecks)
	}
	return router, tokens, matcher
}

func TestInHouseFaceUploadNeverAutoApprovesAndDoesNotUseSmile(t *testing.T) {
	repo := seededFaceReadyRepo(t)
	_, _, matcher := submitForReview(t, repo)
	if matcher.called { t.Fatal("third-party matcher unexpectedly called") }
	approved, _ := repo.CheckRequiredStepsApproved(context.Background(), testProviderID)
	if approved { t.Fatal("provider cannot become verified from a selfie upload") }
}

func TestInHouseFaceManualApprovalRequiresIdentityReviewAndEvidence(t *testing.T) {
	repo := seededFaceReadyRepo(t)
	_, _, _ = submitForReview(t, repo)
	svc := NewService(repo, nil, WithInHouseManualFaceReview())
	ctx := context.Background()
	review := AdminReviewRequest{Step: StepFace, Action: AdminActionApprove, Reason: "Government ID photo and selfie compared by authorized human reviewer."}
	if _, err := svc.AdminReview(ctx, adminReviewTestAdminID, "corr", testProviderID, review); err == nil {
		t.Fatal("face approved before government ID itself was approved")
	}
	identity := repo.steps[testProviderID][StepIdentity]
	identity.Status = StatusApproved
	repo.steps[testProviderID][StepIdentity] = identity

	missingNote := review; missingNote.Reason = ""
	if _, err := svc.AdminReview(ctx, adminReviewTestAdminID, "corr", testProviderID, missingNote); err == nil {
		t.Fatal("manual approval without reviewed evidence notes")
	}
	if _, err := svc.AdminReview(ctx, testProviderID, "corr", testProviderID, review); err == nil {
		t.Fatal("provider can never review their own identity")
	}
	updated, err := svc.AdminReview(ctx, adminReviewTestAdminID, "corr", testProviderID, review)
	if err != nil { t.Fatalf("manual face approval: %v", err) }
	if updated.Status != StatusApproved { t.Fatalf("status=%s", updated.Status) }
	if len(repo.auditRows) == 0 || repo.auditRows[len(repo.auditRows)-1].PerformedBy == nil {
		t.Fatal("manual review must be attributable to an authorized reviewer")
	}
}

func TestInHouseFaceManualApprovalBlocksMissingOrChangedEvidence(t *testing.T) {
	repo := seededFaceReadyRepo(t)
	_, _, _ = submitForReview(t, repo)
	identity := repo.steps[testProviderID][StepIdentity]
	identity.Status = StatusApproved
	repo.steps[testProviderID][StepIdentity] = identity
	review := AdminReviewRequest{Step: StepFace, Action: AdminActionApprove, Reason: "Compared submitted ID photograph against rider selfie."}
	svc := NewService(repo, nil, WithInHouseManualFaceReview())
	ctx := context.Background()
	repo.faceChecks[0].Result = nil
	if _, err := svc.AdminReview(ctx, adminReviewTestAdminID, "corr", testProviderID, review); err == nil {
		t.Fatal("missing pending-review evidence must prevent approval")
	}
	pending := "pending_review"
	repo.faceChecks[0].Result = &pending
	repo.documents[0].FileURL = "local-private://updated-government-id"
	if _, err := svc.AdminReview(ctx, adminReviewTestAdminID, "corr", testProviderID, review); err == nil {
		t.Fatal("updated government ID requires a new selfie")
	}
}

func TestInHouseEvidenceEndpointAdminsOnly(t *testing.T) {
	repo := seededFaceReadyRepo(t)
	router, tokens, _ := submitForReview(t, repo)
	path := "/api/v1/admin/verification/" + testProviderID + "/face-evidence"
	read := func(token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		if token != "" { req.Header.Set("Authorization", "Bearer "+token) }
		w := httptest.NewRecorder(); router.ServeHTTP(w, req); return w
	}
	if w := read(""); w.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous evidence read=%d", w.Code)
	}
	riderToken, _, _ := tokens.GenerateAccessToken(testProviderID, "+2348000000001", "33333333-3333-3333-3333-333333333333")
	if w := read(riderToken); w.Code != http.StatusForbidden {
		t.Fatalf("rider can fetch biometric evidence: %d", w.Code)
	}
	adminToken := mustRoleToken(t, tokens, RolePlatformAdmin)
	w := read(adminToken)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "selfie_reference") ||
		!strings.Contains(w.Body.String(), "government_id_reference") {
		t.Fatalf("admin evidence read=%d body=%s", w.Code, w.Body.String())
	}
}


type fakeFinalCaseRepository struct {
	*fakeVerificationRepository
	approved int
}

func (r *fakeFinalCaseRepository) ApproveInHouseCase(_ context.Context, providerID, reviewerID, notes string) ([]Step, error) {
	r.approved++
	for _, step := range []Step{StepIdentity, StepFace} {
		current := r.steps[providerID][step]
		current.Status = StatusApproved
		current.ReviewerID = &reviewerID
		r.steps[providerID][step] = current
	}
	approved := "manual_approved"
	r.faceChecks[len(r.faceChecks)-1].Result = &approved
	return []Step{StepIdentity, StepFace}, nil
}

func TestOneFinalReviewDossierAndOneAdminDecision(t *testing.T) {
	repo := seededFaceReadyRepo(t)
	wrapped := &fakeFinalCaseRepository{fakeVerificationRepository: repo}
	matcher := &forbiddenExternalMatcher{}
	router, tokens, _ := newInHouseRouter(wrapped, matcher)
	w := doMultipartProviderRequest(t, router, tokens, http.MethodPost,
		"/api/v1/provider/verification/face", nil,
		selfieFiles(testGradientSelfie(t), "image/jpeg"))
	if w.Code != http.StatusOK { t.Fatalf("selfie upload status=%d body=%s", w.Code, w.Body.String()) }
	vehicle := repo.steps[testProviderID][StepVehicle]
	vehicle.Status = StatusApproved
	repo.steps[testProviderID][StepVehicle] = vehicle
	repo.documents = append(repo.documents, VerificationDocument{
		ProviderID: testProviderID, StepID: repo.steps[testProviderID][StepIdentity].ID,
		DocumentType: "profile_photo", FileURL: "local-private://verifications/example/identity/photo.jpg",
	})

	adminToken := mustRoleToken(t, tokens, RolePlatformAdmin)
	path := "/api/v1/admin/verification/" + testProviderID
	get := httptest.NewRequest(http.MethodGet, path+"/dossier", nil)
	get.Header.Set("Authorization", "Bearer "+adminToken)
	view := httptest.NewRecorder()
	router.ServeHTTP(view, get)
	if view.Code != http.StatusOK || !strings.Contains(view.Body.String(), "\"ready_for_review\":true") {
		t.Fatalf("dossier code=%d data=%s", view.Code, view.Body.String())
	}
	if !strings.Contains(view.Body.String(), "\"pending_manual\"") {
		t.Fatal("dossier must disclose unresolved manual biometric checks")
	}

	decision := `{"action":"approve","notes":"Compared the original government ID portrait with the current submitted selfie."}`
	post := httptest.NewRequest(http.MethodPost, path+"/final-review", strings.NewReader(decision))
	post.Header.Set("Authorization", "Bearer "+adminToken)
	post.Header.Set("Content-Type", "application/json")
	result := httptest.NewRecorder()
	router.ServeHTTP(result, post)
	if result.Code != http.StatusOK {
		t.Fatalf("final review status=%d body=%s", result.Code, result.Body.String())
	}
	if wrapped.approved != 1 || repo.steps[testProviderID][StepIdentity].Status != StatusApproved ||
		repo.steps[testProviderID][StepFace].Status != StatusApproved || matcher.called {
		t.Fatalf("single reviewer action failed; count=%d matcher called=%v", wrapped.approved, matcher.called)
	}
}

func TestOneFinalReviewCannotApproveWhileAutomatedChecksFail(t *testing.T) {
	repo := seededFaceReadyRepo(t)
	wrapped := &fakeFinalCaseRepository{fakeVerificationRepository: repo}
	router, tokens, _ := newInHouseRouter(wrapped, &forbiddenExternalMatcher{})
	adminToken := mustRoleToken(t, tokens, RolePlatformAdmin)
	path := "/api/v1/admin/verification/" + testProviderID + "/final-review"
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(
		`{"action":"approve","notes":"Reviewed identity and selfie documentary evidence."}`))
	request.Header.Set("Authorization", "Bearer "+adminToken)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusPreconditionFailed || wrapped.approved != 0 {
		t.Fatalf("incomplete case was approved: status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestFinalReviewRejectsUnauthorizedOrRiderCaller(t *testing.T) {
	repo := seededFaceReadyRepo(t)
	router, tokens, _ := newInHouseRouter(repo, &forbiddenExternalMatcher{})
	path := "/api/v1/admin/verification/" + testProviderID + "/final-review"
	for _, tc := range []struct{ name, token string; expected int } {
		{"anonymous", "", http.StatusUnauthorized},
		{"rider", riderTokenForFinalReview(t, tokens), http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(
				`{"action":"approve","notes":"Checked full original identity and selfie."}`))
			request.Header.Set("Content-Type", "application/json")
			if tc.token != "" { request.Header.Set("Authorization", "Bearer "+tc.token) }
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != tc.expected { t.Fatalf("status=%d want=%d", response.Code, tc.expected) }
		})
	}
}

func riderTokenForFinalReview(t *testing.T, tokens *authusecases.TokenUsecase) string {
	t.Helper()
	token, _, err := tokens.GenerateAccessToken(testProviderID, "+2348000000001", "33333333-3333-3333-3333-333333333333")
	if err != nil { t.Fatal(err) }
	return token
}

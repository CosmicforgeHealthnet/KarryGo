package verification

import (
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

func newInHouseRouter(repo *fakeVerificationRepository, matcher FaceMatcher) (*gin.Engine, *authusecases.TokenUsecase, *captureEvents) {
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

func submitForReview(t *testing.T, repo *fakeVerificationRepository) (*gin.Engine, *authusecases.TokenUsecase, *forbiddenExternalMatcher) {
	t.Helper()
	matcher := &forbiddenExternalMatcher{}
	router, tokens, _ := newInHouseRouter(repo, matcher)
	w := doMultipartProviderRequest(t, router, tokens, http.MethodPost,
		"/api/v1/provider/verification/face", nil, selfieFiles(testJPEG, "image/jpeg"))
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

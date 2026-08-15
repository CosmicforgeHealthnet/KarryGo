package crm

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/vendor-service/internal/platform/authctx"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	"cosmicforge/logistics/shared/go/apperrors"
)

func (e *testEnv) activityRouter(staff, business string) *gin.Engine {
	return e.activityRouterWithLimiter(staff, business, nil)
}

func (e *testEnv) activityRouterWithLimiter(staff, business string, limiter ActivityRateLimiter) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1/vendor")
	group.Use(func(c *gin.Context) {
		authctx.Set(c, authctx.StaffContext{StaffID: staff, VendorID: business, SessionID: "session"})
		c.Next()
	})
	RegisterActivityRoutes(group, NewActivityHandler(e.service, idempotency.NewStore(e.pool, time.Hour), limiter), e.service.FlagSet())
	return router
}

type rejectingActivityLimiter struct{ calls int }

func (l *rejectingActivityLimiter) Allow(context.Context, string, string, int, time.Duration) error {
	l.calls++
	return apperrors.RateLimited("Too many requests.", nil)
}
func activityRequest(t *testing.T, router *gin.Engine, method, path, key string, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

func TestActivityHTTP_IdempotentCreatesReplayAndRejectMismatch(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	env.service.scopes = fixedScopes{"vs_a": {Global: true}}
	profile := env.activityProfile(t, "cus_http_activity", branchA1)
	router := env.activityRouter("vs_a", businessA)
	interactionBody := map[string]any{"customer_profile_ref": profile.ID, "type": InteractionPhoneCall, "direction": DirectionOutbound, "outcome": OutcomeSuccessful, "subject": "HTTP call", "note": "Recorded safely", "visibility": VisibilityBusiness, "occurred_at": env.now}
	first := activityRequest(t, router, http.MethodPost, "/api/v1/vendor/crm/interactions", "interaction-key", interactionBody)
	second := activityRequest(t, router, http.MethodPost, "/api/v1/vendor/crm/interactions", "interaction-key", interactionBody)
	if first.Code != http.StatusCreated || second.Code != http.StatusCreated || first.Body.String() != second.Body.String() {
		t.Fatalf("interaction replay first=%d %s second=%d %s", first.Code, first.Body.String(), second.Code, second.Body.String())
	}
	interactionBody["subject"] = "Different"
	mismatch := activityRequest(t, router, http.MethodPost, "/api/v1/vendor/crm/interactions", "interaction-key", interactionBody)
	if mismatch.Code != http.StatusConflict {
		t.Fatalf("interaction mismatch status=%d body=%s", mismatch.Code, mismatch.Body.String())
	}
	taskBody := map[string]any{"customer_profile_ref": profile.ID, "visibility": VisibilityBusiness, "title": "HTTP task", "description": "Safe description", "priority": PriorityNormal}
	first = activityRequest(t, router, http.MethodPost, "/api/v1/vendor/crm/tasks", "task-key", taskBody)
	second = activityRequest(t, router, http.MethodPost, "/api/v1/vendor/crm/tasks", "task-key", taskBody)
	if first.Code != http.StatusCreated || second.Code != http.StatusCreated || first.Body.String() != second.Body.String() {
		t.Fatalf("task replay first=%d second=%d", first.Code, second.Code)
	}
	var count int
	if err := env.pool.QueryRow(context.Background(), `SELECT count(*) FROM crm_tasks WHERE business_id=$1 AND customer_profile_id=$2`, businessA, profile.ID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("task count=%d err=%v", count, err)
	}
}

func TestActivityHTTP_TransitionsReassignAndStrictInputAreIdempotent(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	env.service.scopes = fixedScopes{"vs_a": {Global: true}}
	ctx := context.Background()
	actor := actorFor(businessA, "vs_a")
	profile := env.activityProfile(t, "cus_http_commands", branchA1)
	env.addStaff(t, "vs_http_assignee", businessA, nil, true)
	router := env.activityRouter("vs_a", businessA)
	task, err := env.service.CreateTask(ctx, actor, globalScope(), basicTask(profile))
	if err != nil {
		t.Fatal(err)
	}
	reassign := map[string]any{"assigned_staff_ref": "vs_http_assignee", "expected_version": task.Version}
	first := activityRequest(t, router, http.MethodPost, "/api/v1/vendor/crm/tasks/"+task.ID+"/reassign", "reassign-key", reassign)
	second := activityRequest(t, router, http.MethodPost, "/api/v1/vendor/crm/tasks/"+task.ID+"/reassign", "reassign-key", reassign)
	if first.Code != http.StatusOK || second.Code != http.StatusOK || first.Body.String() != second.Body.String() {
		t.Fatalf("reassign replay first=%d second=%d", first.Code, second.Code)
	}
	reloaded, err := env.service.GetTask(ctx, actor, globalScope(), task.ID)
	if err != nil {
		t.Fatal(err)
	}
	complete := map[string]any{"expected_version": reloaded.Version}
	first = activityRequest(t, router, http.MethodPost, "/api/v1/vendor/crm/tasks/"+task.ID+"/complete", "complete-key", complete)
	second = activityRequest(t, router, http.MethodPost, "/api/v1/vendor/crm/tasks/"+task.ID+"/complete", "complete-key", complete)
	if first.Code != http.StatusOK || second.Code != http.StatusOK || first.Body.String() != second.Body.String() {
		t.Fatalf("complete replay first=%d second=%d", first.Code, second.Code)
	}
	bad := map[string]any{"customer_profile_ref": profile.ID, "type": InteractionNote, "direction": DirectionInternal, "outcome": OutcomeNone, "subject": "Bad field", "note": "plain", "visibility": VisibilityBusiness, "email": "must-not-be-accepted"}
	rec := activityRequest(t, router, http.MethodPost, "/api/v1/vendor/crm/interactions", "strict-key", bad)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("unknown sensitive field status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestActivityHTTP_FeatureFlagsRemoveSurface(t *testing.T) {
	flags := allFlagsOn()
	flags.InteractionsEnabled = false
	flags.TasksEnabled = false
	env := newTestEnv(t, flags)
	router := env.activityRouter("vs_a", businessA)
	for _, path := range []string{"/api/v1/vendor/crm/interactions", "/api/v1/vendor/crm/tasks"} {
		rec := activityRequest(t, router, http.MethodGet, path, "", map[string]any{})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("disabled path %s status=%d", path, rec.Code)
		}
	}
}

func TestActivityHTTP_WriteCommandsUseExistingRateLimiter(t *testing.T) {
	env := newTestEnv(t, allFlagsOn())
	env.service.scopes = fixedScopes{"vs_a": {Global: true}}
	profile := env.activityProfile(t, "cus_http_rate_limit", branchA1)
	limiter := &rejectingActivityLimiter{}
	router := env.activityRouterWithLimiter("vs_a", businessA, limiter)
	rec := activityRequest(t, router, http.MethodPost, "/api/v1/vendor/crm/interactions", "limited-key", map[string]any{
		"customer_profile_ref": profile.ID,
		"type":                 InteractionNote,
		"direction":            DirectionInternal,
		"outcome":              OutcomeNone,
		"subject":              "Limited",
		"note":                 "plain text",
		"visibility":           VisibilityBusiness,
	})
	if rec.Code != http.StatusTooManyRequests || limiter.calls != 1 {
		t.Fatalf("rate-limited command status=%d calls=%d body=%s", rec.Code, limiter.calls, rec.Body.String())
	}
	var count int
	if err := env.pool.QueryRow(context.Background(), `SELECT count(*) FROM crm_interactions WHERE business_id=$1 AND customer_profile_id=$2`, businessA, profile.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("rate-limited interaction count=%d err=%v", count, err)
	}
}

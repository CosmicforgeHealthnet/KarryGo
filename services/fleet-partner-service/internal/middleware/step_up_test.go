package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/auth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/stepup"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

// recordingVerifier accepts exactly one reference and records what it was
// asked to check.
type recordingVerifier struct {
	valid string
	calls [][5]string
}

func (v *recordingVerifier) Verify(_ context.Context, reference, identityID, sessionID, organizationID, actionClass string) error {
	v.calls = append(v.calls, [5]string{reference, identityID, sessionID, organizationID, actionClass})
	if reference != v.valid {
		return errorcodes.Forbidden(errorcodes.StepUpRequired, "This action requires additional verification.")
	}
	return nil
}

func stepUpEngine(verifier stepup.Verifier) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(respond.ErrorHandler())
	engine.Use(func(c *gin.Context) {
		c.Set(auth.ClaimsContextKey, auth.Claims{Subject: "cid_1", SessionID: "ses_1"})
		c.Set(TenantContextKey, tenant.Context{OrganizationID: "org_1", OrganizationUserID: "oru_1", ActorType: tenant.ActorTypeOrganizationUser, ActorID: "cid_1"})
		c.Next()
	})
	engine.Use(RequireStepUp(verifier))
	group := engine.Group("/fleet-partner/v1")
	ok := func(c *gin.Context) { c.Status(http.StatusNoContent) }
	group.POST("/roles", ok)
	group.GET("/roles", ok)
	// A data-dependent check inside a handler uses the proof the middleware
	// attached to the request context.
	group.POST("/exports", func(c *gin.Context) {
		if err := stepup.Require(c.Request.Context(), stepup.ActionSensitiveExport); err != nil {
			respond.Error(c, err)
			return
		}
		c.Status(http.StatusNoContent)
	})
	return engine
}

func serveStepUp(engine *gin.Engine, method, path, reference string) (int, map[string]any) {
	request := httptest.NewRequest(method, path, nil)
	if reference != "" {
		request.Header.Set(stepup.HeaderReference, reference)
	}
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, request)
	body := map[string]any{}
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	return recorder.Code, body
}

func TestRequireStepUp_ProtectedRouteNeedsAGrantForItsActionClass(t *testing.T) {
	verifier := &recordingVerifier{valid: "chl_ok"}
	engine := stepUpEngine(verifier)

	if code, _ := serveStepUp(engine, "GET", "/fleet-partner/v1/roles", ""); code != http.StatusNoContent {
		t.Fatalf("unprotected route = %d, want 204", code)
	}
	if len(verifier.calls) != 0 {
		t.Fatalf("unprotected route consulted the verifier: %v", verifier.calls)
	}

	code, body := serveStepUp(engine, "POST", "/fleet-partner/v1/roles", "")
	errBody, _ := body["error"].(map[string]any)
	if code != http.StatusForbidden || errBody["code"] != string(errorcodes.StepUpRequired) {
		t.Fatalf("protected route without grant = %d %v", code, body)
	}
	details, _ := errBody["details"].([]any)
	if len(details) != 1 || details[0].(map[string]any)["message"] != stepup.ActionAccessManage {
		t.Fatalf("refusal must name the action class: %v", body)
	}
	if len(verifier.calls) != 0 {
		t.Fatal("a missing reference must be refused without a lookup")
	}

	if code, _ := serveStepUp(engine, "POST", "/fleet-partner/v1/roles", "chl_wrong"); code != http.StatusForbidden {
		t.Fatalf("wrong grant = %d, want 403", code)
	}
	if code, _ := serveStepUp(engine, "POST", "/fleet-partner/v1/roles", "chl_ok"); code != http.StatusNoContent {
		t.Fatalf("valid grant = %d, want 204", code)
	}
	last := verifier.calls[len(verifier.calls)-1]
	if last != [5]string{"chl_ok", "cid_1", "ses_1", "org_1", stepup.ActionAccessManage} {
		t.Fatalf("verifier was asked %v; identity, session and organization must come from the token and tenant", last)
	}

	if code, _ := serveStepUp(engine, "POST", "/fleet-partner/v1/exports", ""); code != http.StatusForbidden {
		t.Fatalf("handler-level check without grant = %d, want 403", code)
	}
	if code, _ := serveStepUp(engine, "POST", "/fleet-partner/v1/exports", "chl_ok"); code != http.StatusNoContent {
		t.Fatalf("handler-level check with grant = %d, want 204", code)
	}
}

func TestRequireStepUp_NilVerifierFailsClosed(t *testing.T) {
	engine := stepUpEngine(nil)
	code, body := serveStepUp(engine, "POST", "/fleet-partner/v1/roles", "chl_anything")
	if code != http.StatusServiceUnavailable {
		t.Fatalf("protected route with no verifier = %d %v, want 503", code, body)
	}
	if code, _ := serveStepUp(engine, "GET", "/fleet-partner/v1/roles", ""); code != http.StatusNoContent {
		t.Fatalf("unprotected route with no verifier = %d, want 204", code)
	}
}

func TestStepUpRequire_WithoutMiddlewareRefuses(t *testing.T) {
	if err := stepup.Require(context.Background(), stepup.ActionSensitiveExport); err == nil {
		t.Fatal("a call that never passed the step-up middleware must be refused")
	}
}

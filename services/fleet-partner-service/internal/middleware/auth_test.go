package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/auth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/respond"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

type fakeResolver struct {
	membership tenant.Membership
	err        error
}

func (f fakeResolver) ResolveMembership(context.Context, string) (tenant.Membership, error) {
	return f.membership, f.err
}

type fakeSessionChecker struct {
	status string
	err    error
}

func (f fakeSessionChecker) Status(context.Context, string) (string, error) {
	return f.status, f.err
}

func withClaims(claims auth.Claims) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(auth.ClaimsContextKey, claims)
		c.Next()
	}
}

func testRouter(resolver tenant.MembershipResolver, sessions SessionStatusChecker) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(respond.ErrorHandler())
	router.Use(withClaims(auth.Claims{Subject: "usr_1", SessionID: "ses_1", Role: "organization_user", Service: "fleet-partner-service", Type: auth.TokenTypeAccess}))
	router.Use(ResolveTenant(resolver, sessions))
	router.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })
	return router
}

// A session revoked mid-lifetime (DELETE /sessions/:sessionId, credential
// reset, reuse-detected family revocation) must stop authorizing requests on
// its very next request, not merely once its own access-token TTL elapses.
func TestResolveTenant_RevokedSessionRejectedEvenWithActiveMembership(t *testing.T) {
	resolver := fakeResolver{membership: tenant.Membership{
		OrganizationID: "org_1", OrganizationUserID: "ou_1", Status: tenant.MembershipStatusActive,
	}}
	router := testRouter(resolver, fakeSessionChecker{status: "REVOKED"})

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/x", nil))
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session: status = %d, want 401", resp.Code)
	}
}

func TestResolveTenant_ActiveSessionAndMembershipPasses(t *testing.T) {
	resolver := fakeResolver{membership: tenant.Membership{
		OrganizationID: "org_1", OrganizationUserID: "ou_1", Status: tenant.MembershipStatusActive,
	}}
	router := testRouter(resolver, fakeSessionChecker{status: "ACTIVE"})

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/x", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("active session: status = %d, want 200, body=%s", resp.Code, resp.Body.String())
	}
}

// A nil checker preserves pre-existing behavior exactly, matching every
// other optional dependency in this composition root.
func TestResolveTenant_NilSessionCheckerSkipsEnforcement(t *testing.T) {
	resolver := fakeResolver{membership: tenant.Membership{
		OrganizationID: "org_1", OrganizationUserID: "ou_1", Status: tenant.MembershipStatusActive,
	}}
	router := testRouter(resolver, nil)

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/x", nil))
	if resp.Code != http.StatusOK {
		t.Fatalf("nil session checker: status = %d, want 200", resp.Code)
	}
}

// A lookup error must fail closed: an unprovable session is never treated as
// valid, the same posture the authorizer's own nil-check follows.
func TestResolveTenant_SessionLookupErrorFailsClosed(t *testing.T) {
	resolver := fakeResolver{membership: tenant.Membership{
		OrganizationID: "org_1", OrganizationUserID: "ou_1", Status: tenant.MembershipStatusActive,
	}}
	router := testRouter(resolver, fakeSessionChecker{err: context.DeadlineExceeded})

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/x", nil))
	if resp.Code == http.StatusOK {
		t.Fatalf("session lookup error must not pass through as valid, got %d", resp.Code)
	}
}

// A revoked session is rejected before the membership resolver is even
// consulted -- session revocation must not depend on membership state.
func TestResolveTenant_RevokedSessionRejectedEvenIfResolverWouldFail(t *testing.T) {
	resolver := fakeResolver{err: tenant.ErrMembershipNotFound}
	router := testRouter(resolver, fakeSessionChecker{status: "COMPROMISED"})

	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, httptest.NewRequest(http.MethodGet, "/x", nil))
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("compromised session: status = %d, want 401", resp.Code)
	}
}

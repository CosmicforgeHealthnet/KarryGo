package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/auth"

	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
)

type versionedResolver struct{ version int64 }

func (r *versionedResolver) ResolveMembership(context.Context, string) (tenant.Membership, error) {
	return tenant.Membership{OrganizationID: "org_rv", OrganizationUserID: "oru_rv",
		Status: tenant.MembershipStatusActive, RoleVersion: r.version}, nil
}

// TestResolveTenantPublishesRoleVersion: authorization caches key on the role
// version, so the tenant middleware must publish the membership's current
// version. Before this fix it was never set and every handler used 1, so a
// revoked role kept authorizing until the cache entry expired.
func TestResolveTenantPublishesRoleVersion(t *testing.T) {
	gin.SetMode(gin.TestMode)
	resolver := &versionedResolver{version: 7}
	var seen []int64
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set(auth.ClaimsContextKey, auth.Claims{Subject: "cid_rv", SessionID: "ses_rv"})
		c.Next()
	})
	engine.Use(ResolveTenant(resolver, nil))
	engine.GET("/probe", func(c *gin.Context) {
		value, ok := c.Get(TenantRoleVersionKey)
		if !ok {
			t.Fatal("role version not published")
		}
		seen = append(seen, value.(int64))
		c.Status(http.StatusNoContent)
	})
	for _, version := range []int64{7, 8, 0} {
		resolver.version = version
		engine.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/probe", nil))
	}
	if len(seen) != 3 || seen[0] != 7 || seen[1] != 8 || seen[2] != 1 {
		t.Fatalf("published role versions = %v, want [7 8 1]", seen)
	}
}

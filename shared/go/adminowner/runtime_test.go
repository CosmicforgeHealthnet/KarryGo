package adminowner

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestOwnerResourcesDefaultDisabledWithoutCredentialAccess(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, profile := range Profiles() {
		t.Run(profile.Domain, func(t *testing.T) {
			t.Setenv(profile.Prefix+"_ADMIN_RESOURCE_READ_ENABLED", "false")
			t.Setenv(profile.Prefix+"_ADMIN_CONTEXT_PUBLISH_ENABLED", "false")
			t.Setenv(profile.Prefix+"_ADMIN_CASE_SOURCE_READ_ENABLED", "false")
			t.Setenv(profile.Prefix+"_ADMIN_CASE_CONTEXT_PUBLISH_ENABLED", "false")
			t.Setenv(profile.Prefix+"_ADMIN_CASE_SOURCE_SECRET_FILE", "missing-must-not-be-opened")
			t.Setenv(profile.Prefix+"_ADMIN_RESOURCE_SECRET_FILE", "missing-must-not-be-opened")
			h, err := FromEnvironment(context.Background(), nil, nil, profile.Definitions, profile.Owner, profile.Prefix, profile.Path)
			if err != nil || h.Enabled || h.PublishEnabled {
				t.Fatal("disabled startup required credentials or enabled a capability", err)
			}
			engine := gin.New()
			h.Register(engine)
			if h.Cases != nil {
				for _, entry := range []struct{ method, path string }{{"GET", h.Cases.Path + "/proofs/recoveries/record-1/CASE/record-1"}, {"GET", h.Cases.Path + "/authorization-contexts/bindings/media-case:recoveries/record-1?purpose=case-1"}, {"POST", h.Cases.Path + "/authorization-contexts"}} {
					out := httptest.NewRecorder()
					engine.ServeHTTP(out, httptest.NewRequest(entry.method, entry.path, nil))
					if out.Code != 503 {
						t.Fatal("disabled case endpoint did not fail closed", out.Code)
					}
				}
			}
			for kind := range profile.Definitions {
				for _, path := range []string{profile.Path + "/resources/" + kind, profile.Path + "/authorization-contexts/bindings/" + kind + "/record-1?purpose=case-1"} {
					res := httptest.NewRecorder()
					engine.ServeHTTP(res, httptest.NewRequest("GET", path, nil))
					if res.Code != 503 || strings.Contains(res.Body.String(), "known_source_dimensions") || strings.Contains(res.Body.String(), "items") {
						t.Fatalf("disabled capability disclosed data: %d %s", res.Code, res.Body.String())
					}
				}
			}
		})
	}
}

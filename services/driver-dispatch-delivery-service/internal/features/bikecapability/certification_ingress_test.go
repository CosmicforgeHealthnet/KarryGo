package bikecapability

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// routesOf returns every path a registrar mounts on a fresh group.
func routesOf(t *testing.T, register func(*gin.RouterGroup)) []string {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	register(engine.Group("/api/v1/internal"))
	paths := []string{}
	for _, route := range engine.Routes() {
		paths = append(paths, route.Method+" "+route.Path)
	}
	return paths
}

func contains(paths []string, fragment string) bool {
	for _, path := range paths {
		if strings.Contains(path, fragment) {
			return true
		}
	}
	return false
}

// TestCertificationIngressIsMountedSeparatelyFromTheCapabilityRead pins the
// split between two surfaces with two different callers.
//
// They used to share one registrar and therefore one HMAC boundary, which meant
// this service could only be certified by a caller holding Vendor's key, and
// that a deployment with no Vendor secret had no certification ingress at all.
// Products requiring certification would then fail closed permanently, with
// nothing able to change that state.
func TestCertificationIngressIsMountedSeparatelyFromTheCapabilityRead(t *testing.T) {
	handler := &Handler{service: &Service{}}

	capabilityRoutes := routesOf(t, func(group *gin.RouterGroup) {
		RegisterInternal(group, handler)
	})
	if !contains(capabilityRoutes, "/bikes/:id/capabilities") {
		t.Fatalf("the capability read is missing: %v", capabilityRoutes)
	}
	if contains(capabilityRoutes, "certification-events") {
		t.Fatalf("the certification callback is still on Vendor's boundary: %v", capabilityRoutes)
	}

	certificationRoutes := routesOf(t, func(group *gin.RouterGroup) {
		RegisterCertificationIngress(group, handler)
	})
	if !contains(certificationRoutes, "POST /api/v1/internal/verification/certification-events") {
		t.Fatalf("the certification callback is not mounted: %v", certificationRoutes)
	}
	if contains(certificationRoutes, "/bikes/:id/capabilities") {
		t.Fatalf("the capability read leaked onto the certification boundary: %v", certificationRoutes)
	}
}

// TestCertificationIngressPathMatchesWhatVerificationCalls guards the one
// string both services have to agree on.
//
// verification-compliance-service stores this path as a constant of its own. If
// either side moves it the delivery becomes a 404 that the outbox retries for
// hours and then dead-letters, which reads as a Verification outage rather than
// a renamed route.
func TestCertificationIngressPathMatchesWhatVerificationCalls(t *testing.T) {
	const agreedPath = "/api/v1/internal/verification/certification-events"

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterCertificationIngress(engine.Group("/api/v1/internal"), &Handler{service: &Service{}})

	// A request to the agreed path must reach a handler rather than the
	// router's 404. The handler itself will refuse the empty body; what is
	// under test is that the route exists at that exact path.
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, agreedPath, strings.NewReader("{}"))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)
	if recorder.Code == http.StatusNotFound {
		t.Fatalf("nothing is mounted at %s; Verification's deliveries would 404", agreedPath)
	}
}

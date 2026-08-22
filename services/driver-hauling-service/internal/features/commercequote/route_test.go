package commercequote

// The quote endpoint's path is half of a cross-service contract that no
// compiler checks. Vendor Service builds the full URL as
// baseURL + apiBase + commercequote.QuotePath and this service must register
// exactly that; a mismatch is a 404 on every call with no other symptom.
//
// This test pins the half that lives here against the shared constant. The
// other half is pinned in vendor-service's own composition-root test.

import (
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/shared/go/commercequote"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// haulingQuoteAPIBase is the base vendor-service is configured with for this
// service, and the base cmd/main.go mounts every hauling route under.
const haulingQuoteAPIBase = "/api/v1/hauling"

func TestQuoteRouteMatchesTheSharedContract(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()

	// db must be non-nil for registration to proceed; the boundary only
	// touches it when a request arrives, and no request is made here.
	Register(
		router.Group(haulingQuoteAPIBase),
		NewHandler(New(Options{Store: newFakeStore()})),
		&pgxpool.Pool{},
		serviceauth.NewVerifier(
			serviceauth.Secrets{commercequote.SignatureCaller: []byte("x")}, 0,
		),
		[]string{commercequote.SignatureCaller},
	)

	want := haulingQuoteAPIBase + commercequote.QuotePath

	var found bool

	for _, route := range router.Routes() {
		if route.Path == want && route.Method == "POST" {
			found = true
		}
	}

	if !found {
		var paths []string

		for _, route := range router.Routes() {
			paths = append(paths, route.Method+" "+route.Path)
		}

		t.Fatalf("no route at %q; registered: %v", want, paths)
	}
}

// The caller this service allowlists must be the one vendor-service signs as.
func TestAllowlistedCallerMatchesTheSharedContract(t *testing.T) {
	if commercequote.SignatureCaller != "vendor-service" {
		t.Fatalf("shared caller identity changed to %q; the HAULING_SERVICE_SECRETS entry and the router allowlist must change with it",
			commercequote.SignatureCaller)
	}
}

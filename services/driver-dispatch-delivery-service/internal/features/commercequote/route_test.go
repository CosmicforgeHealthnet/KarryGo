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

	"cosmicforge/logistics/services/dispatch-delivery-service/internal/features/authority"
	"cosmicforge/logistics/shared/go/commercequote"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// dispatchQuoteAPIBase is the base vendor-service is configured with for this
// service. It is stated here so a change to either side fails a test rather
// than a production call.
const dispatchQuoteAPIBase = "/api/v1"

func TestQuoteRouteMatchesTheSharedContract(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()

	Register(
		router.Group(dispatchQuoteAPIBase+"/internal"),
		NewHandler(New(Options{Store: newFakeStore(), Live: fakeLive{}})),
		serviceauth.NewVerifier(
			serviceauth.Secrets{commercequote.SignatureCaller: []byte("x")}, 0,
		),
		authority.NewPostgresNonceClaimer((*pgxpool.Pool)(nil)),
		[]string{commercequote.SignatureCaller},
	)

	want := dispatchQuoteAPIBase + commercequote.QuotePath

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
		t.Fatalf("shared caller identity changed to %q; the router allowlist and every deployment's secret entry must change with it",
			commercequote.SignatureCaller)
	}
}

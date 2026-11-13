package fleetauthority

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/serviceauth"
)

// FLEET-10.2 against real PostgreSQL: Fleet's opaque organization ids are
// accepted for sponsored provisioning, a sponsored provider can be associated
// with the organization that sponsored it (and no other) before platform
// verification, and the provider's own decline is final.
func TestSponsoredProviderAssociationAndDeclinePostgres(t *testing.T) {
	dsn := os.Getenv("HAULING_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("HAULING_TEST_DATABASE_URL is required for authority integration tests")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	secret := []byte("integration-only-hmac-secret")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpx.ErrorHandler())
	root := router.Group("/api/v1/hauling")
	handler := New(pool, Flags{Enabled: true})
	Register(root, handler, serviceauth.NewVerifier(map[string][]byte{"fleet-partner-service": secret}, 5*time.Minute), []string{"fleet-partner-service"})
	signer := sharedauth.NewTokenSigner([]byte("integration-provider-token-secret"))
	RegisterProviderConsent(root.Group("/provider/organization-associations"), handler,
		sharedauth.BearerMiddleware(signer, "truck_provider", "hauling"))

	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")
	org := "org_" + suffix
	email := "sponsored-" + suffix[:12] + "@example.invalid"
	phone := fmt.Sprintf("+23480%08d", time.Now().UnixNano()%100000000)
	sponsor := map[string]any{"organizationId": org, "legalName": "Chidi Haul", "emailNormalized": email,
		"phoneE164": phone, "serviceCategory": "HAULING", "onboardingMode": "ORGANIZATION_SPONSORED",
		"providerMode": "ORGANIZATION_MANAGED"}
	key := "sponsor-" + suffix
	created := authorityCall(t, router, secret, http.MethodPost, "/api/v1/hauling/internal/marketplace/providers/sponsored", sponsor, key, uuid.NewString(), http.StatusCreated)
	providerID := nestedString(t, created, "providerId")
	replayed := authorityCall(t, router, secret, http.MethodPost, "/api/v1/hauling/internal/marketplace/providers/sponsored", sponsor, key, uuid.NewString(), http.StatusOK)
	if nestedString(t, replayed, "providerId") != providerID {
		t.Fatal("sponsored replay returned a different provider")
	}
	resolved := authorityCall(t, router, secret, http.MethodPost, "/api/v1/hauling/internal/marketplace/providers/resolve",
		map[string]any{"emailNormalized": email, "phoneE164": phone}, "", uuid.NewString(), http.StatusOK)
	if nestedString(t, resolved, "outcome") != "MATCHED" || nestedString(t, resolved, "managedByOrgId") != org {
		t.Fatalf("resolve after sponsor: %#v", resolved)
	}

	other := "org_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	authorityCall(t, router, secret, http.MethodPost, "/api/v1/hauling/internal/marketplace/provider-links",
		map[string]any{"organizationId": other, "membershipId": "pmb_" + suffix[:10], "providerId": providerID},
		"assoc-other-"+suffix, uuid.NewString(), http.StatusUnprocessableEntity)
	link := authorityCall(t, router, secret, http.MethodPost, "/api/v1/hauling/internal/marketplace/provider-links",
		map[string]any{"organizationId": org, "membershipId": "pmb_" + suffix[:12], "providerId": providerID},
		"assoc:pmb_"+suffix[:12], uuid.NewString(), http.StatusCreated)
	linkID := nestedString(t, link, "linkId")

	code, declined := providerDecision(t, router, signer, providerID, linkID, "decline")
	if code != http.StatusOK || nestedString(t, declined, "status") != "REJECTED" {
		t.Fatalf("decline: %d %#v", code, declined)
	}
	if code, _ := providerDecision(t, router, signer, providerID, linkID, "accept"); code != http.StatusConflict {
		t.Fatalf("accept after decline: %d", code)
	}
	request := signedAuthorityRequest(t, secret, http.MethodGet, "/api/v1/hauling/internal/marketplace/provider-links/"+linkID, nil, "", uuid.NewString())
	request.Header.Set("X-Organization-ID", org)
	readBack := serve(router, request)
	if readBack.Code != http.StatusOK || !strings.Contains(readBack.Body.String(), `"status":"REJECTED"`) {
		t.Fatalf("fleet read-back: %d %s", readBack.Code, readBack.Body.String())
	}
}

func serve(router http.Handler, request *http.Request) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

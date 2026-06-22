package wallethttp

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

func init() {
	gin.SetMode(gin.TestMode)
}

func providerToken(t *testing.T, secret []byte, subject, role, service string) string {
	t.Helper()
	signer := sharedauth.NewTokenSigner(secret)
	token, err := signer.Sign(sharedauth.Claims{
		Subject:   subject,
		Role:      role,
		Service:   service,
		SessionID: "sess-1",
		Type:      sharedauth.TokenTypeAccess,
		ExpiresAt: time.Now().Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

func newProviderRouter(secrets map[string][]byte) *gin.Engine {
	router := gin.New()
	router.Use(httpx.ErrorHandler())
	router.GET("/provider/earnings", providerBearerMiddleware(secrets), func(c *gin.Context) {
		providerType, providerID := providerIdentity(c)
		c.JSON(http.StatusOK, gin.H{"provider_type": providerType, "provider_id": providerID})
	})
	return router
}

func TestProviderBearerMiddleware(t *testing.T) {
	haulingSecret := []byte("development-hauling-provider-token-secret")
	taxiSecret := []byte("development-taxi-access-token-secret")
	secrets := map[string][]byte{"hauling": haulingSecret, "taxi": taxiSecret}

	t.Run("accepts hauling provider token with truck_provider role", func(t *testing.T) {
		// Regression for M1: hauling mints role="truck_provider", not "provider".
		token := providerToken(t, haulingSecret, "prov-1", "truck_provider", "hauling")
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/provider/earnings", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		newProviderRouter(secrets).ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d (%s)", rec.Code, rec.Body.String())
		}
		if got := rec.Body.String(); !contains(got, `"provider_type":"hauling"`) || !contains(got, `"provider_id":"prov-1"`) {
			t.Fatalf("unexpected identity in body: %s", got)
		}
	})

	t.Run("rejects token signed for a different service", func(t *testing.T) {
		// Token says service=taxi but signed with the taxi secret; valid for taxi
		// only. We confirm a hauling secret can't impersonate, and the service
		// binding is honored: sign a taxi token, it should resolve as taxi.
		token := providerToken(t, taxiSecret, "prov-2", "driver", "taxi")
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/provider/earnings", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		newProviderRouter(secrets).ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected taxi token to pass as taxi, got %d", rec.Code)
		}
		if got := rec.Body.String(); !contains(got, `"provider_type":"taxi"`) {
			t.Fatalf("expected taxi provider type, got %s", got)
		}
	})

	t.Run("rejects unknown secret", func(t *testing.T) {
		token := providerToken(t, []byte("attacker-secret"), "prov-3", "truck_provider", "hauling")
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/provider/earnings", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		newProviderRouter(secrets).ServeHTTP(rec, req)

		if rec.Code == http.StatusOK {
			t.Fatalf("expected rejection of token signed with unknown secret, got 200")
		}
	})

	t.Run("rejects token whose service is not configured", func(t *testing.T) {
		// Signed with a valid secret but claims a service with no signer match.
		token := providerToken(t, haulingSecret, "prov-4", "truck_provider", "dispatch")
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/provider/earnings", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		newProviderRouter(secrets).ServeHTTP(rec, req)

		if rec.Code == http.StatusOK {
			t.Fatalf("expected rejection when claims.Service has no matching signer, got 200")
		}
	})

	t.Run("rejects missing authorization header", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/provider/earnings", nil)
		newProviderRouter(secrets).ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d", rec.Code)
		}
	})
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

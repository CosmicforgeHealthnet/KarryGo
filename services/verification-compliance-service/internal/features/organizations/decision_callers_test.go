package organizations

import (
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"cosmicforge/logistics/shared/go/serviceauth"
)

// The submitting service must not record a decision on a company it
// submitted. The refusal happens before the nonce is consumed, so it needs no
// database.
func TestSubmitterCannotDecideOrganization(t *testing.T) {
	secret := []byte("fleet-secret")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	verifier := serviceauth.NewVerifier(map[string][]byte{"fleet-partner-service": secret}, 5*time.Minute)
	Register(router.Group("/v"), New(nil, true), verifier,
		[]string{"fleet-partner-service", "admin-backoffice-service"}, []string{"admin-backoffice-service"})
	path := "/v/internal/organizations/" + uuid.NewString() + "/submissions/" + uuid.NewString() + "/decisions"
	body := map[string]any{"status": "VERIFIED", "restrictionStatus": "NONE", "expectedVersion": 1, "issues": []any{}}
	if got := verificationCall(t, router, secret, http.MethodPost, path, body, "k", uuid.NewString()); got.Code != http.StatusForbidden {
		t.Fatalf("fleet decision status=%d, want 403", got.Code)
	}
}

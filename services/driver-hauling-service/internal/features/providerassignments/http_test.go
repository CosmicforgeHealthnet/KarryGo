package providerassignments

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	providerauthusecases "cosmicforge/logistics/services/hauling-service/internal/features/provider_auth/usecases"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/serviceauth"
)

func TestProviderBearerIngressRelaysSelectedProviderWithHMAC(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sharedSecret := []byte("fleet-hmac-secret")
	var relayed map[string]any
	fleet := gin.New()
	fleet.Use(serviceauth.Middleware(serviceauth.NewVerifier(
		map[string][]byte{"driver-hauling-service": sharedSecret}, 5*time.Minute)))
	fleet.POST("/internal/fleet-partner/v1/private-assignments/:assignmentId/accept", func(c *gin.Context) {
		if c.Param("assignmentId") != "pas_private_1" || c.GetHeader("Idempotency-Key") != "response-1" {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		if err := c.ShouldBindJSON(&relayed); err != nil {
			c.AbortWithStatus(http.StatusBadRequest)
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"assignmentId": c.Param("assignmentId"), "status": "ACCEPTED"}})
	})
	fleetServer := httptest.NewServer(fleet)
	defer fleetServer.Close()

	tokenSecret := []byte("provider-token-secret")
	authService := providerauthusecases.NewAuthService(providerauthusecases.Options{
		AccessTokenSecret: tokenSecret})
	api := gin.New()
	root := api.Group("/api/v1/hauling")
	Register(root, authService, NewClient(fleetServer.URL, "driver-hauling-service", sharedSecret, time.Second))
	token, err := sharedauth.NewTokenSigner(tokenSecret).Sign(sharedauth.Claims{
		Subject: "provider-selected", Role: providerauthusecases.ProviderRole,
		Service: providerauthusecases.ProviderService, SessionID: "session-1",
		ExpiresAt: time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	body := `{"clientResponseId":"response-1","expectedVersion":2,"providerId":"provider-attacker"}`
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/hauling/provider/company-assignments/pas_private_1/accept", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if relayed["providerId"] != "provider-selected" || relayed["expectedVersion"] != float64(2) {
		t.Fatalf("unsafe provider relay: %+v", relayed)
	}
	if _, ok := relayed["clientResponseId"]; ok {
		t.Fatalf("client response id leaked into body: %+v", relayed)
	}
	var envelope map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
}

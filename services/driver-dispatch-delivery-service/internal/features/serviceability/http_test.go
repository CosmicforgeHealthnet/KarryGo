package serviceability

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
	"github.com/gin-gonic/gin"
)

func TestCustomerAuthenticationOwnsServiceabilityQuote(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	store := &memoryStore{}
	service := serviceFor(store,
		fakeRoutes{result: RouteResult{DistanceMeters: 5000, DurationSeconds: 900, Source: "roads", EvidenceVersion: "v1"}},
		fakeZones{true, true, false, true}, false, time.Now().UTC())
	signer := sharedauth.NewTokenSigner([]byte("customer-secret"))
	engine := gin.New()
	engine.Use(httpx.ErrorHandler())
	RegisterRoutes(engine, signer, NewHandler(service))
	body, _ := json.Marshal(validRequest())

	request := httptest.NewRequest(http.MethodPost, "/api/v1/customer/dispatch/serviceability", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "customer-owned")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("missing auth status=%d body=%s", response.Code, response.Body.String())
	}

	customerID := "9bcd1c37-edcc-4206-a2a8-37622f999999"
	token, err := signer.Sign(sharedauth.Claims{Subject: customerID, Role: "customer", Service: "customer", SessionID: "session-1", ExpiresAt: time.Now().Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodPost, "/api/v1/customer/dispatch/serviceability", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", "customer-owned")
	request.Header.Set("Authorization", "Bearer "+token)
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("authenticated status=%d body=%s", response.Code, response.Body.String())
	}
	if _, ok := store.byKey[customerID+":customer-owned"]; !ok {
		t.Fatalf("quote was not scoped to authenticated customer: %+v", store.byKey)
	}
}

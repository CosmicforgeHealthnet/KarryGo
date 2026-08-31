package food

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/shared/go/serviceauth"
)

func discoveryRouter(env *testEnv, secrets serviceauth.Secrets) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterInternalRoutes(router, NewInternalHandler(env.service), serviceauth.NewVerifier(secrets, 5*time.Minute), allFlagsOn())
	return router
}

func signedDiscoveryRequest(t *testing.T, method, path, caller string, secret []byte, body []byte, nonce string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	nonce += "-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	req.Header.Set(serviceauth.HeaderNonce, nonce)
	if len(body) != 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if err := serviceauth.SignRequest(req, caller, secret, body, time.Now()); err != nil {
		t.Fatal(err)
	}
	return req
}

func TestSection9CInternalSecurityAndDiscoveryContract(t *testing.T) {
	env := newTestEnv(t, allFlagsOn(), nil, nil)
	ctx := context.Background()
	actor := actorFor(businessA, staffA)
	menu := env.seedMenu(t, businessA, staffA, branchA1, "RESTAURANT")
	section := env.seedSection(t, businessA, staffA, menu, "Discovery")
	item, err := env.service.CreateItem(ctx, actor, globalScope(), menu.ID, section.ID, CreateItemInput{Name: "DST Breakfast", BasePriceMinor: 1200, DietaryCodes: []string{"VEGAN"}, AllergenCodes: []string{"MILK"}})
	if err != nil {
		t.Fatal(err)
	}
	menu, err = env.service.SetMenuStatus(ctx, actor, globalScope(), menu.ID, StatusActive, menu.Version)
	if err != nil {
		t.Fatal(err)
	}
	item, err = env.service.UpdateItem(ctx, actor, globalScope(), item.ID, UpdateItemInput{Status: stringPtr(StatusActive), ExpectedVersion: item.Version})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.SetItemAvailability(ctx, actor, globalScope(), item.ID, SetAvailabilityInput{Status: OperationalAvailable, ExpectedVersion: 0}); err != nil {
		t.Fatal(err)
	}
	if _, err = env.service.DiscoverCatalogue(ctx, DiscoveryOptions{BranchID: branchA1, Capability: "RESTAURANT", SellableOnly: true}); err != nil {
		t.Fatalf("direct discovery: %v", err)
	}

	secret := []byte("section9c-test-secret")
	router := discoveryRouter(env, serviceauth.Secrets{"customer-service": secret, "attacker-service": []byte("wrong")})
	path := "/internal/vendor/v1/food/branches/" + branchA1 + "/revision?capability=RESTAURANT"
	withoutAuth := httptest.NewRequest(http.MethodGet, path, nil)
	withoutAuthResponse := httptest.NewRecorder()
	router.ServeHTTP(withoutAuthResponse, withoutAuth)
	if withoutAuthResponse.Code != http.StatusUnauthorized {
		t.Fatalf("missing HMAC status=%d", withoutAuthResponse.Code)
	}

	attacker := signedDiscoveryRequest(t, http.MethodGet, path, "attacker-service", []byte("wrong"), nil, "9c-attacker")
	attackerResponse := httptest.NewRecorder()
	router.ServeHTTP(attackerResponse, attacker)
	if attackerResponse.Code != http.StatusForbidden {
		t.Fatalf("unapproved caller status=%d", attackerResponse.Code)
	}

	valid := signedDiscoveryRequest(t, http.MethodGet, path, "customer-service", secret, nil, "9c-valid")
	validResponse := httptest.NewRecorder()
	router.ServeHTTP(validResponse, valid)
	if validResponse.Code != http.StatusOK {
		t.Fatalf("valid revision status=%d body=%s", validResponse.Code, validResponse.Body.String())
	}
	var revisionEnvelope struct {
		Data DiscoveryRevision `json:"data"`
	}
	if err = json.Unmarshal(validResponse.Body.Bytes(), &revisionEnvelope); err != nil || revisionEnvelope.Data.BranchRef != branchA1 {
		t.Fatalf("revision response=%s err=%v", validResponse.Body.String(), err)
	}

	replayResponse := httptest.NewRecorder()
	router.ServeHTTP(replayResponse, valid)
	if replayResponse.Code != http.StatusUnauthorized {
		t.Fatalf("replayed nonce status=%d", replayResponse.Code)
	}

	cataloguePath := "/internal/vendor/v1/food/branches/" + branchA1 + "/catalogue?capability=RESTAURANT&q=breakfast&dietary_code=VEGAN&exclude_allergen=PEANUT"
	catalogue := signedDiscoveryRequest(t, http.MethodGet, cataloguePath, "customer-service", secret, nil, "9c-catalogue")
	catalogueResponse := httptest.NewRecorder()
	router.ServeHTTP(catalogueResponse, catalogue)
	if catalogueResponse.Code != http.StatusOK {
		t.Fatalf("catalogue status=%d body=%s", catalogueResponse.Code, catalogueResponse.Body.String())
	}
	var catalogueEnvelope struct {
		Data DiscoverySnapshot `json:"data"`
	}
	if err = json.Unmarshal(catalogueResponse.Body.Bytes(), &catalogueEnvelope); err != nil || len(catalogueEnvelope.Data.Menus) == 0 {
		t.Fatalf("catalogue response=%s err=%v", catalogueResponse.Body.String(), err)
	}
	unchangedPath := "/internal/vendor/v1/food/branches/" + branchA1 + "/catalogue?capability=RESTAURANT&since_revision=" + strconv.FormatInt(catalogueEnvelope.Data.CatalogueRevision, 10)
	unchanged := signedDiscoveryRequest(t, http.MethodGet, unchangedPath, "customer-service", secret, nil, "9c-unchanged")
	unchangedResponse := httptest.NewRecorder()
	router.ServeHTTP(unchangedResponse, unchanged)
	if unchangedResponse.Code != http.StatusOK || !bytes.Contains(unchangedResponse.Body.Bytes(), []byte(`"unchanged":true`)) {
		t.Fatalf("unchanged catalogue status=%d body=%s", unchangedResponse.Code, unchangedResponse.Body.String())
	}

	body := []byte(`{"item_ref":"` + item.ID + `"}`)
	pricePath := "/internal/vendor/v1/food/branches/" + branchA1 + "/pricing/resolve"
	price := signedDiscoveryRequest(t, http.MethodPost, pricePath, "customer-service", secret, body, "9c-price")
	priceResponse := httptest.NewRecorder()
	router.ServeHTTP(priceResponse, price)
	if priceResponse.Code != http.StatusOK {
		t.Fatalf("price status=%d body=%s", priceResponse.Code, priceResponse.Body.String())
	}
	if !bytes.Contains(priceResponse.Body.Bytes(), []byte(`"catalogue_revision"`)) {
		t.Fatalf("price response omitted catalogue revision: %s", priceResponse.Body.String())
	}

	altered := signedDiscoveryRequest(t, http.MethodPost, pricePath, "customer-service", secret, body, "9c-altered")
	altered.Body = http.NoBody
	alteredResponse := httptest.NewRecorder()
	router.ServeHTTP(alteredResponse, altered)
	if alteredResponse.Code != http.StatusUnauthorized {
		t.Fatalf("altered signed body status=%d", alteredResponse.Code)
	}

	wrongBranch := signedDiscoveryRequest(t, http.MethodGet, "/internal/vendor/v1/food/branches/"+branchA2+"/items/"+item.ID+"/sellability", "customer-service", secret, nil, "9c-idor")
	wrongBranchResponse := httptest.NewRecorder()
	router.ServeHTTP(wrongBranchResponse, wrongBranch)
	if wrongBranchResponse.Code < http.StatusBadRequest {
		t.Fatalf("cross-branch item read unexpectedly succeeded: %d", wrongBranchResponse.Code)
	}
}

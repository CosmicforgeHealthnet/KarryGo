package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"cosmicforge/logistics/services/vendor-service/internal/config"
	"cosmicforge/logistics/services/vendor-service/internal/features/checkout"
	"cosmicforge/logistics/services/vendor-service/internal/platform/idempotency"
	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

// Section 17B HTTP contract, driven through the real production registration
// helper rather than a parallel test-only router.

const section17BCustomerSecret = "section17b-customer-access-secret"

func (e *s17bEnv) httpRouter() *gin.Engine {
	e.t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()

	// The same platform middleware serviceapp.Run installs. Without the error
	// handler the shared bearer middleware's abort would record an error that
	// nothing renders, and a test router would report 200 for a request
	// production refuses.
	router.Use(httpx.RequestID())
	router.Use(httpx.Recovery())
	router.Use(httpx.ErrorHandler())

	registerSection17BCheckoutRoutes(
		router,
		config.Config{
			CustomerAccessTokenSecret: []byte(section17BCustomerSecret),
			Flags: config.FeatureFlags{
				CommerceCheckoutEnabled: true,
			},
		},
		e.checkout,
		idempotency.NewStore(e.pool, time.Hour),
	)

	return router
}

func section17BCustomerToken(t *testing.T, customerID string) string {
	t.Helper()

	signer := sharedauth.NewTokenSigner([]byte(section17BCustomerSecret))

	now := time.Now().UTC()

	token, err := signer.Sign(sharedauth.Claims{
		Subject:   customerID,
		Role:      "customer",
		Service:   "customer",
		SessionID: "s17b-http-session",
		Type:      sharedauth.TokenTypeAccess,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}

	return token
}

type section17BResponse struct {
	Success bool                       `json:"success"`
	Data    checkout.Session           `json:"data"`
	Error   map[string]json.RawMessage `json:"error"`
}

func section17BDo(
	t *testing.T,
	router *gin.Engine,
	method string,
	path string,
	token string,
	ifMatch string,
	body any,
) (*httptest.ResponseRecorder, section17BResponse) {
	t.Helper()

	var payload []byte

	if body != nil {
		encoded, err := json.Marshal(body)

		if err != nil {
			t.Fatal(err)
		}

		payload = encoded
	}

	request := httptest.NewRequest(method, path, bytes.NewReader(payload))

	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}

	if ifMatch != "" {
		request.Header.Set("If-Match", ifMatch)
	}

	// Every customer command on this base requires an idempotency key
	// (Section 17A's convention, which Section 17B reuses unchanged). A fresh
	// key per call keeps each assertion about checkout rather than about
	// replay.
	if method != http.MethodGet {
		request.Header.Set("Idempotency-Key", uuid.NewString())
	}

	request.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)

	var decoded section17BResponse

	if recorder.Body.Len() > 0 {
		_ = json.Unmarshal(recorder.Body.Bytes(), &decoded)
	}

	return recorder, decoded
}

// TestSection17BHTTPValidatesThroughTheRealRouter walks the whole customer
// surface: start, read, cancel.
func TestSection17BHTTPValidatesThroughTheRealRouter(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)

	item := env.seedShopItem(s17bStockOptions{
		Business:   env.businessA,
		Staff:      env.staffA,
		Branch:     env.branchA1,
		Name:       "HTTP Stock",
		Quantity:   5,
		PriceMinor: 60000,
	})

	customer := env.customer("http")
	current := env.seedShopCart(customer, s17bCartLine{Item: item, Quantity: 2})

	router := env.httpRouter()
	token := section17BCustomerToken(t, customer)

	recorder, response := section17BDo(
		t,
		router,
		http.MethodPost,
		"/api/v1/commerce/checkout-sessions",
		token,
		strconv.FormatInt(current.Version, 10),
		map[string]string{"cart_id": current.ID},
	)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("start status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	if !response.Success ||
		response.Data.Status != checkout.StatusValidated {
		t.Fatalf("body=%s", recorder.Body.String())
	}

	// The ETag carries the session version, which is what a later optimistic
	// command will have to match.
	if recorder.Header().Get("ETag") == "" {
		t.Fatal("no ETag on the checkout session response")
	}

	// The wire shape must never leak the customer identity back out.
	var envelope struct {
		Data map[string]json.RawMessage `json:"data"`
	}

	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}

	if _, present := envelope.Data["customer_id"]; present {
		t.Fatal("the checkout session serialises a customer identity")
	}

	sessionID := response.Data.ID

	readRecorder, readResponse := section17BDo(
		t,
		router,
		http.MethodGet,
		"/api/v1/commerce/checkout-sessions/"+sessionID,
		token,
		"",
		nil,
	)

	if readRecorder.Code != http.StatusOK ||
		readResponse.Data.ID != sessionID {
		t.Fatalf("read status=%d body=%s", readRecorder.Code, readRecorder.Body.String())
	}

	cancelRecorder, cancelResponse := section17BDo(
		t,
		router,
		http.MethodPost,
		"/api/v1/commerce/checkout-sessions/"+sessionID+"/cancel",
		token,
		"",
		nil,
	)

	if cancelRecorder.Code != http.StatusOK ||
		cancelResponse.Data.Status != checkout.StatusCancelled {
		t.Fatalf("cancel status=%d body=%s",
			cancelRecorder.Code, cancelRecorder.Body.String())
	}

	if available := env.availableStock(
		env.businessA,
		env.branchA1,
		item.InventoryItemID,
	); available != 5 {
		t.Fatalf("available after HTTP cancel=%d want=5", available)
	}
}

// TestSection17BHTTPRefusesUnauthenticatedAndMalformedCommands pins the
// gates a client hits before any authority is consulted.
func TestSection17BHTTPRefusesUnauthenticatedAndMalformedCommands(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)

	item := env.seedShopItem(s17bStockOptions{
		Business:   env.businessA,
		Staff:      env.staffA,
		Branch:     env.branchA1,
		Name:       "Guard Stock",
		Quantity:   5,
		PriceMinor: 10000,
	})

	customer := env.customer("guard")
	current := env.seedShopCart(customer, s17bCartLine{Item: item, Quantity: 1})

	router := env.httpRouter()
	token := section17BCustomerToken(t, customer)

	version := strconv.FormatInt(current.Version, 10)

	// No token at all.
	if recorder, _ := section17BDo(
		t,
		router,
		http.MethodPost,
		"/api/v1/commerce/checkout-sessions",
		"",
		version,
		map[string]string{"cart_id": current.ID},
	); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d", recorder.Code)
	}

	// A token from the wrong issuer.
	wrongSigner := sharedauth.NewTokenSigner([]byte("some-other-service-secret"))

	now := time.Now().UTC()

	foreign, err := wrongSigner.Sign(sharedauth.Claims{
		Subject:   customer,
		Role:      "customer",
		Service:   "customer",
		SessionID: "s17b-foreign-session",
		Type:      sharedauth.TokenTypeAccess,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}

	if recorder, _ := section17BDo(
		t,
		router,
		http.MethodPost,
		"/api/v1/commerce/checkout-sessions",
		foreign,
		version,
		map[string]string{"cart_id": current.ID},
	); recorder.Code != http.StatusUnauthorized {
		t.Fatalf("foreign-token status=%d", recorder.Code)
	}

	// If-Match is required: a checkout against a cart version the client has
	// not seen is the stale-checkout defect this section exists to prevent.
	if recorder, _ := section17BDo(
		t,
		router,
		http.MethodPost,
		"/api/v1/commerce/checkout-sessions",
		token,
		"",
		map[string]string{"cart_id": current.ID},
	); recorder.Code != http.StatusUnprocessableEntity {
		t.Fatalf("missing If-Match status=%d", recorder.Code)
	}

	// A stale cart version is a conflict, not a silent revalidation.
	if recorder, _ := section17BDo(
		t,
		router,
		http.MethodPost,
		"/api/v1/commerce/checkout-sessions",
		token,
		strconv.FormatInt(current.Version-1, 10),
		map[string]string{"cart_id": current.ID},
	); recorder.Code != http.StatusConflict {
		t.Fatalf("stale If-Match status=%d", recorder.Code)
	}

	// Unknown body fields are refused rather than ignored: a client sending
	// customer_id or a price must not be quietly accepted.
	if recorder, _ := section17BDo(
		t,
		router,
		http.MethodPost,
		"/api/v1/commerce/checkout-sessions",
		token,
		version,
		map[string]any{
			"cart_id":     current.ID,
			"customer_id": "someone-else",
		},
	); recorder.Code != http.StatusBadRequest {
		t.Fatalf("unknown-field status=%d", recorder.Code)
	}

	// The shared customer-command middleware requires an idempotency key, and
	// Section 17B does not opt out of it.
	bare := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/commerce/checkout-sessions",
		bytes.NewReader([]byte(`{"cart_id":"`+current.ID+`"}`)),
	)

	bare.Header.Set("Authorization", "Bearer "+token)
	bare.Header.Set("If-Match", version)
	bare.Header.Set("Content-Type", "application/json")

	bareRecorder := httptest.NewRecorder()
	router.ServeHTTP(bareRecorder, bare)

	if bareRecorder.Code != http.StatusBadRequest {
		t.Fatalf("missing Idempotency-Key status=%d body=%s",
			bareRecorder.Code, bareRecorder.Body.String())
	}

	// Nothing above may have held any stock.
	if available := env.availableStock(
		env.businessA,
		env.branchA1,
		item.InventoryItemID,
	); available != 5 {
		t.Fatalf("available=%d want=5: a refused request held stock", available)
	}
}

// TestSection17BHTTPCannotReadAnotherCustomersSession is the isolation gate at
// the transport boundary, where a real attacker would be.
func TestSection17BHTTPCannotReadAnotherCustomersSession(t *testing.T) {
	env := newSection17BEnv(t, 10*time.Minute)

	item := env.seedShopItem(s17bStockOptions{
		Business:   env.businessA,
		Staff:      env.staffA,
		Branch:     env.branchA1,
		Name:       "Isolation Stock",
		Quantity:   5,
		PriceMinor: 10000,
	})

	owner := env.customer("http_owner")
	intruder := env.customer("http_intruder")

	current := env.seedShopCart(owner, s17bCartLine{Item: item, Quantity: 1})

	router := env.httpRouter()

	recorder, response := section17BDo(
		t,
		router,
		http.MethodPost,
		"/api/v1/commerce/checkout-sessions",
		section17BCustomerToken(t, owner),
		strconv.FormatInt(current.Version, 10),
		map[string]string{"cart_id": current.ID},
	)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("start status=%d body=%s", recorder.Code, recorder.Body.String())
	}

	intruderToken := section17BCustomerToken(t, intruder)

	if readRecorder, _ := section17BDo(
		t,
		router,
		http.MethodGet,
		"/api/v1/commerce/checkout-sessions/"+response.Data.ID,
		intruderToken,
		"",
		nil,
	); readRecorder.Code != http.StatusNotFound {
		t.Fatalf("cross-customer read status=%d", readRecorder.Code)
	}

	if cancelRecorder, _ := section17BDo(
		t,
		router,
		http.MethodPost,
		"/api/v1/commerce/checkout-sessions/"+response.Data.ID+"/cancel",
		intruderToken,
		"",
		nil,
	); cancelRecorder.Code != http.StatusNotFound {
		t.Fatalf("cross-customer cancel status=%d", cancelRecorder.Code)
	}

	// The owner's hold survived the attempt.
	if available := env.availableStock(
		env.businessA,
		env.branchA1,
		item.InventoryItemID,
	); available != 4 {
		t.Fatalf("available=%d want=4", available)
	}
}

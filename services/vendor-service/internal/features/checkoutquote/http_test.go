package checkoutquote

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	sharedauth "cosmicforge/logistics/shared/go/auth"
	"cosmicforge/logistics/shared/go/httpx"
)

type httpQuoteStore struct {
	source SourceSnapshot
	quote  Quote
}

func (s *httpQuoteStore) LoadSource(_ context.Context, customerID, sessionID, planID string, _ time.Time) (SourceSnapshot, error) {
	if customerID != s.source.CustomerID || sessionID != s.source.CheckoutSessionID || planID != s.source.FulfilmentPlanID {
		return SourceSnapshot{}, ErrSourceNotQuotable
	}
	return s.source, nil
}

func (s *httpQuoteStore) Save(_ context.Context, quote Quote, _ time.Time) (Quote, error) {
	quote.ID = "cq_http"
	quote.QuoteVersion = 1
	quote.QuoteFingerprint, _ = QuoteFingerprint(quote)
	s.quote = quote
	return quote, nil
}

func (s *httpQuoteStore) Get(_ context.Context, customerID, quoteID string, _ time.Time) (Quote, error) {
	if quoteID != s.quote.ID || customerID != s.quote.CustomerID {
		return Quote{}, ErrQuoteNotFound
	}
	return s.quote, nil
}

func (s *httpQuoteStore) Approve(_ context.Context, customerID, quoteID, fingerprint string, now time.Time) (Quote, error) {
	if quoteID != s.quote.ID || customerID != s.quote.CustomerID {
		return Quote{}, ErrQuoteNotFound
	}
	if fingerprint != s.quote.QuoteFingerprint {
		return Quote{}, ErrFingerprintMismatch
	}
	s.quote.State = StateApproved
	s.quote.ApprovedAt = &now
	return s.quote, nil
}

func quoteHTTPFixture(t *testing.T) (*gin.Engine, *httpQuoteStore, string) {
	t.Helper()
	now := time.Now().UTC()
	store := &httpQuoteStore{source: sourceFixture(now)}
	service, err := NewService(Options{Store: store, QuoteTTL: 5 * time.Minute, Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	signer := sharedauth.NewTokenSigner([]byte("section17d-http-secret"))
	token, err := signer.Sign(sharedauth.Claims{
		Subject: store.source.CustomerID, Role: "customer", Service: "customer",
		SessionID: "customer-session", Type: sharedauth.TokenTypeAccess,
		IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Hour).Unix(),
	})
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(httpx.RequestID(), httpx.Recovery(), httpx.ErrorHandler())
	RegisterCustomerRoutes(router.Group("/api/v1/commerce"), service, signer, nil, true)
	return router, store, token
}

func performQuoteRequest(router *gin.Engine, method, path, token, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	return recorder
}

func TestQuoteHTTPRejectsCustomerCommercialOverrides(t *testing.T) {
	router, _, token := quoteHTTPFixture(t)
	recorder := performQuoteRequest(router, http.MethodPost,
		"/api/v1/commerce/checkout/checkout-1/quotes", token,
		`{"fulfilment_plan_id":"plan-1","quoted_total_minor":1,"customer_ref":"someone-else"}`)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("override status=%d body=%s", recorder.Code, recorder.Body.String())
	}
}

func TestQuoteHTTPCreateReadAndApproveExactFingerprint(t *testing.T) {
	router, store, token := quoteHTTPFixture(t)
	create := performQuoteRequest(router, http.MethodPost,
		"/api/v1/commerce/checkout/checkout-1/quotes", token,
		`{"fulfilment_plan_id":"plan-1"}`)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	var envelope struct {
		Data Quote `json:"data"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Data.CustomerID != "customer-1" || envelope.Data.QuotedTotalMinor != 1200 || envelope.Data.QuoteFingerprint == "" {
		t.Fatalf("incomplete agreement response: %+v", envelope.Data)
	}
	bad := performQuoteRequest(router, http.MethodPost,
		"/api/v1/commerce/quotes/cq_http/approve", token,
		`{"quote_fingerprint":"wrong"}`)
	if bad.Code != http.StatusConflict {
		t.Fatalf("mismatched fingerprint status=%d body=%s", bad.Code, bad.Body.String())
	}
	approve := performQuoteRequest(router, http.MethodPost,
		"/api/v1/commerce/quotes/cq_http/approve", token,
		`{"quote_fingerprint":"`+store.quote.QuoteFingerprint+`"}`)
	if approve.Code != http.StatusOK {
		t.Fatalf("approve status=%d body=%s", approve.Code, approve.Body.String())
	}
	read := performQuoteRequest(router, http.MethodGet,
		"/api/v1/commerce/quotes/cq_http", token, "")
	if read.Code != http.StatusOK {
		t.Fatalf("read status=%d body=%s", read.Code, read.Body.String())
	}
}

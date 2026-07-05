package http

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/api-gateway/internal/features/partneraccount"
	"cosmicforge/logistics/services/api-gateway/internal/pkg/pricing"
)

// testPricingConfig mirrors the spec defaults used across quote tests.
var testPricingConfig = pricing.Config{
	BaseKobo:          50000,
	StandardPerKmKobo: 15000,
	ExpressPerKmKobo:  25000,
	LargeMultiplier:   1.5,
}

// fakeAccountRepo is a minimal partneraccount.AccountRepository for the
// RequireActiveAccount tests. Only FindByID is exercised.
type fakeAccountRepo struct {
	account *partneraccount.Account
}

func (r *fakeAccountRepo) CreateAccount(context.Context, partneraccount.CreateAccountParams) (partneraccount.Account, error) {
	return partneraccount.Account{}, nil
}
func (r *fakeAccountRepo) FindByEmail(context.Context, string) (*partneraccount.Account, string, error) {
	return nil, "", nil
}
func (r *fakeAccountRepo) FindByID(_ context.Context, _ string) (*partneraccount.Account, error) {
	return r.account, nil
}
func (r *fakeAccountRepo) UpdatePlan(context.Context, string, string) (bool, error) {
	return true, nil
}
func (r *fakeAccountRepo) UpdateStatus(context.Context, string, string) (bool, error) {
	return true, nil
}

// newQuoteRouter builds a router with API-key auth, the pricing config, and an
// optional account repo (for active-account enforcement).
func newQuoteRouter(t *testing.T, accounts partneraccount.AccountRepository) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	svc, _ := newTestService()
	RegisterPartnerRoutes(r, Dependencies{
		KeyRepo:     newTestRepo(t),
		Deliveries:  svc,
		Pricing:     testPricingConfig,
		AccountRepo: accounts,
	})
	return r
}

func postQuote(r *gin.Engine, body string, withKey bool) *httptest.ResponseRecorder {
	return requestBody(r, http.MethodPost, "/api/v1/partner/v1/quotes", body, withKey)
}

type quoteData struct {
	Success bool `json:"success"`
	Data    struct {
		DistanceKM float64 `json:"distance_km"`
		Currency   string  `json:"currency"`
		AmountKobo int64   `json:"amount_kobo"`
		Tier       string  `json:"tier"`
		Size       string  `json:"size"`
	} `json:"data"`
}

func decodeQuote(t *testing.T, w *httptest.ResponseRecorder) quoteData {
	t.Helper()
	var q quoteData
	if err := json.Unmarshal(w.Body.Bytes(), &q); err != nil {
		t.Fatalf("unmarshal: %v (%s)", err, w.Body.String())
	}
	return q
}

const stdBody = `{"pickup":{"lat":6.5244,"lng":3.3792},"dropoff":{"lat":6.6018,"lng":3.3515},"tier":"standard","size":"small"}`

func TestQuoteStandardSuccess(t *testing.T) {
	r := newQuoteRouter(t, nil)
	w := postQuote(r, stdBody, true)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
	q := decodeQuote(t, w)
	if !q.Success || q.Data.Currency != "NGN" || q.Data.Tier != "standard" || q.Data.Size != "small" {
		t.Fatalf("unexpected: %s", w.Body.String())
	}
	if q.Data.DistanceKM <= 0 || q.Data.AmountKobo < testPricingConfig.BaseKobo {
		t.Fatalf("distance/amount invalid: %+v", q.Data)
	}
}

func TestQuoteExpressHigherThanStandard(t *testing.T) {
	r := newQuoteRouter(t, nil)
	std := decodeQuote(t, postQuote(r, stdBody, true))
	expBody := `{"pickup":{"lat":6.5244,"lng":3.3792},"dropoff":{"lat":6.6018,"lng":3.3515},"tier":"express","size":"small"}`
	exp := decodeQuote(t, postQuote(r, expBody, true))
	if exp.Data.AmountKobo <= std.Data.AmountKobo {
		t.Fatalf("express %d must be > standard %d", exp.Data.AmountKobo, std.Data.AmountKobo)
	}
}

func TestQuoteLargeAppliesMultiplier(t *testing.T) {
	r := newQuoteRouter(t, nil)
	small := decodeQuote(t, postQuote(r, stdBody, true))
	largeBody := `{"pickup":{"lat":6.5244,"lng":3.3792},"dropoff":{"lat":6.6018,"lng":3.3515},"tier":"standard","size":"large"}`
	large := decodeQuote(t, postQuote(r, largeBody, true))
	if large.Data.AmountKobo <= small.Data.AmountKobo {
		t.Fatalf("large %d must be > small %d", large.Data.AmountKobo, small.Data.AmountKobo)
	}
}

func TestQuoteDefaultsTierAndSize(t *testing.T) {
	r := newQuoteRouter(t, nil)
	// no tier/size fields at all
	body := `{"pickup":{"lat":6.5244,"lng":3.3792},"dropoff":{"lat":6.6018,"lng":3.3515}}`
	q := decodeQuote(t, postQuote(r, body, true))
	if q.Data.Tier != "standard" || q.Data.Size != "small" {
		t.Fatalf("defaults wrong: tier=%q size=%q", q.Data.Tier, q.Data.Size)
	}
}

func TestQuoteInvalidLatitude(t *testing.T) {
	r := newQuoteRouter(t, nil)
	body := `{"pickup":{"lat":200,"lng":3.3792},"dropoff":{"lat":6.6018,"lng":3.3515}}`
	if w := postQuote(r, body, true); w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", w.Code, w.Body.String())
	}
}

func TestQuoteInvalidLongitude(t *testing.T) {
	r := newQuoteRouter(t, nil)
	body := `{"pickup":{"lat":6.5244,"lng":200},"dropoff":{"lat":6.6018,"lng":3.3515}}`
	if w := postQuote(r, body, true); w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", w.Code, w.Body.String())
	}
}

func TestQuoteMissingCoordinate(t *testing.T) {
	r := newQuoteRouter(t, nil)
	body := `{"pickup":{"lat":6.5244},"dropoff":{"lat":6.6018,"lng":3.3515}}`
	if w := postQuote(r, body, true); w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", w.Code, w.Body.String())
	}
}

func TestQuoteInvalidTier(t *testing.T) {
	r := newQuoteRouter(t, nil)
	body := `{"pickup":{"lat":6.5244,"lng":3.3792},"dropoff":{"lat":6.6018,"lng":3.3515},"tier":"jet"}`
	w := postQuote(r, body, true)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", w.Code, w.Body.String())
	}
}

func TestQuoteInvalidSize(t *testing.T) {
	r := newQuoteRouter(t, nil)
	body := `{"pickup":{"lat":6.5244,"lng":3.3792},"dropoff":{"lat":6.6018,"lng":3.3515},"size":"huge"}`
	w := postQuote(r, body, true)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", w.Code, w.Body.String())
	}
}

// --- auth protection ---------------------------------------------------------

func TestQuoteMissingAPIKey(t *testing.T) {
	r := newQuoteRouter(t, nil)
	if w := postQuote(r, stdBody, false); w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestQuoteInvalidAPIKey(t *testing.T) {
	r := newQuoteRouter(t, nil)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/partner/v1/quotes", strings.NewReader(stdBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "sk_test_wrong_key_value_1234567890")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 (body=%s)", w.Code, w.Body.String())
	}
}

func TestQuoteInactiveAccountRejected(t *testing.T) {
	suspended := &partneraccount.Account{ID: "partner-1", Status: partneraccount.StatusSuspended, Plan: partneraccount.PlanSandbox}
	r := newQuoteRouter(t, &fakeAccountRepo{account: suspended})
	w := postQuote(r, stdBody, true)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (body=%s)", w.Code, w.Body.String())
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &env)
	if env.Error.Code != "partner_account_inactive" {
		t.Fatalf("code = %q, want partner_account_inactive", env.Error.Code)
	}
}

func TestQuoteActiveAccountSucceeds(t *testing.T) {
	active := &partneraccount.Account{ID: "partner-1", Status: partneraccount.StatusActive, Plan: partneraccount.PlanSandbox}
	r := newQuoteRouter(t, &fakeAccountRepo{account: active})
	if w := postQuote(r, stdBody, true); w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", w.Code, w.Body.String())
	}
}

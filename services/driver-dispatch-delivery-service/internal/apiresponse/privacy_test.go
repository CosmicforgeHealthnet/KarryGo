package apiresponse

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"cosmicforge/logistics/shared/go/providercompat"
	"github.com/gin-gonic/gin"
)

type privacyReader struct {
	mode string
	err  error
}

func (r privacyReader) ReadMembershipSummary(context.Context, string, string) (providercompat.Summary, error) {
	return providercompat.Summary{ProviderMode: r.mode}, r.err
}

type corporatePrivacyReader struct {
	providercompat.Summary
	context providercompat.CorporateContext
	err     error
}

func (r corporatePrivacyReader) ReadMembershipSummary(context.Context, string, string) (providercompat.Summary, error) {
	return r.Summary, nil
}

func (r corporatePrivacyReader) ReadCorporateContext(context.Context, string, string) (providercompat.CorporateContext, error) {
	return r.context, r.err
}

func TestProviderViewsConcealManagedFinanceAndTerminalContact(t *testing.T) {
	for _, terminal := range []bool{false, true} {
		status := "in_progress"
		if terminal {
			status = "completed"
		}
		data := map[string]any{"trip": map[string]any{"status": status, "fare_amount": int64(9007199254740993), "receiver_phone": "PRIVATE_CONTACT"}, "proof": map[string]any{"receiver_phone": "PRIVATE_CONTACT"}, "recovery": map[string]any{"additional_fee_kobo": int64(500)}}
		for _, managed := range []bool{false, true} {
			out, err := json.Marshal(providerView(data, managed))
			if err != nil {
				t.Fatal(err)
			}
			if managed && (strings.Contains(string(out), "9007199254740993") || strings.Contains(string(out), ":500")) {
				t.Fatal("managed finance leaked")
			}
			if !managed && !strings.Contains(string(out), "9007199254740993") {
				t.Fatal("independent fare lost or rounded")
			}
			if terminal && strings.Contains(string(out), "PRIVATE_CONTACT") {
				t.Fatal("terminal contact leaked")
			}
		}
		if data["trip"].(map[string]any)["fare_amount"] != int64(9007199254740993) {
			t.Fatal("domain data mutated")
		}
	}
	proof := providerView(map[string]any{"photo_ref": "private-ref", "submitted_at": "now", "receiver_phone": "PRIVATE_CONTACT"}, false)
	b, _ := json.Marshal(proof)
	if strings.Contains(string(b), "PRIVATE_CONTACT") {
		t.Fatal("standalone proof contact leaked")
	}
}

func TestProviderViewsUseTypedFinancialVisibilityPolicy(t *testing.T) {
	policy := providercompat.FinancialVisibilityPolicy{ShowProviderShare: true}
	data := map[string]any{
		"provider_share_minor":      int64(1200),
		"organization_settlement":   int64(7000),
		"platform_commission_minor": int64(800),
	}
	out, err := json.Marshal(providerViewWithPolicy(data, true, &policy))
	if err != nil {
		t.Fatal(err)
	}
	text := string(out)
	if !strings.Contains(text, "1200") {
		t.Fatal("authorized provider share was removed")
	}
	if strings.Contains(text, "7000") || strings.Contains(text, "800") {
		t.Fatalf("organization/platform finance leaked: %s", text)
	}
}

func TestProviderPrivacyAuthorityFailurePrecedesDomainAction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, tc := range []struct {
		mode  string
		err   error
		allow bool
	}{{providercompat.ModeIndependent, nil, true}, {providercompat.ModeOrganizationManaged, nil, true}, {"", errors.New("secret"), false}, {"unknown", nil, false}} {
		r := gin.New()
		r.Use(ProviderPrivacy(privacyReader{tc.mode, tc.err}))
		mutated := false
		r.POST("/action", func(c *gin.Context) {
			c.Set("dispatch_rider_id", "provider-a")
			hook, _ := c.Get("dispatch_provider_privacy_check")
			if err := hook.(func(*gin.Context) error)(c); err != nil {
				c.AbortWithStatus(503)
				return
			}
			mutated = true
			c.Status(200)
		})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("POST", "/action", nil))
		if mutated != tc.allow {
			t.Fatal("visibility authority bypass")
		}
	}
}

func TestProviderPrivacyPrefersCanonicalCorporateContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	reader := corporatePrivacyReader{context: providercompat.CorporateContext{
		ProviderID:        "provider-a",
		IsManagedProvider: true,
		FinancialVisibilityPolicy: providercompat.FinancialVisibilityPolicy{
			ShowProviderShare: true,
		},
	}}
	r := gin.New()
	r.Use(ProviderPrivacy(reader))
	r.POST("/action", func(c *gin.Context) {
		c.Set("dispatch_rider_id", "provider-a")
		hook, _ := c.Get("dispatch_provider_privacy_check")
		if err := hook.(func(*gin.Context) error)(c); err != nil {
			c.AbortWithStatus(503)
			return
		}
		if _, ok := c.Get("dispatch_financial_visibility_policy"); !ok {
			t.Fatal("typed visibility policy was not stored")
		}
		c.Status(200)
	})
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("POST", "/action", nil))
	if w.Code != 200 {
		t.Fatalf("status=%d", w.Code)
	}
}

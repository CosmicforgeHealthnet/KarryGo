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

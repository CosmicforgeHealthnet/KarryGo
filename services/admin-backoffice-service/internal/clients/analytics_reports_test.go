package clients

import (
	"context"
	"cosmicforge/logistics/shared/go/serviceauth"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func asReport[T any](v T, err error) (any, error) { return v, err }
func TestTenAnalyticsContractsUseFixedSignedOwnerPaths(t *testing.T) {
	secret := []byte(strings.Repeat("a", 32))
	seen := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := serviceauth.NewVerifier(serviceauth.Secrets{"admin-backoffice-service": secret}, time.Minute).VerifyRequest(r); err != nil {
			t.Error(err)
			w.WriteHeader(401)
			return
		}
		if r.Method != "GET" || r.Header.Get("Authorization") != "" || r.Header.Get("X-HQ-Key-ID") != "" {
			t.Error("wrong method or HQ credential propagation")
		}
		seen[r.URL.Path]++
		data := any([]map[string]any{{"authority": "TAXI", "date": "2026-10-01T00:00:00Z", "owner_secret": "SECRET"}})
		switch r.URL.Path {
		case "/api/v1/analytics/network/provider-quality/TAXI/provider-1":
			data = map[string]any{"authority": "TAXI", "provider_id": "provider-1", "owner_secret": "SECRET"}
		case "/api/v1/analytics/network/readiness/branches/branch-1":
			data = map[string]any{"branch_id": "branch-1", "owner_secret": "SECRET"}
		case "/api/v1/analytics/network/supply", "/api/v1/analytics/network/demand-forecast":
			data = map[string]any{"authority": "TAXI", "zone": "Z1", "product_code": "TAXI_RIDE", "owner_secret": "SECRET"}
		}
		json.NewEncoder(w).Encode(map[string]any{"success": true, "data": data})
	}))
	defer server.Close()
	client, err := NewAnalytics(server.URL, secret)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	ctx := context.Background()
	calls := []struct {
		path string
		call func() (any, error)
	}{
		{"mission-success", func() (any, error) { return asReport(client.MissionSuccess(ctx, "TAXI", 7, "correlation-1")) }},
		{"recovery", func() (any, error) { return asReport(client.Recovery(ctx, "TAXI", 7, "correlation-1")) }},
		{"capability-mismatches", func() (any, error) { return asReport(client.CapabilityMismatches(ctx, "TAXI", 7, "correlation-1")) }},
		{"declines", func() (any, error) { return asReport(client.Declines(ctx, "TAXI", 7, "correlation-1")) }},
		{"demand", func() (any, error) { return asReport(client.Demand(ctx, "TAXI", 7, "correlation-1")) }},
		{"provider-quality/TAXI/provider-1", func() (any, error) {
			return asReport(client.ProviderQuality(ctx, "TAXI", "provider-1", 7, "correlation-1"))
		}},
		{"readiness/branches/branch-1", func() (any, error) { return asReport(client.BranchReadiness(ctx, "branch-1", 7, "correlation-1")) }},
		{"supply", func() (any, error) { return asReport(client.Supply(ctx, "TAXI", "Z1", "TAXI_RIDE", "correlation-1")) }},
		{"mission-outcomes", func() (any, error) {
			return asReport(client.MissionOutcomes(ctx, "TAXI", "TAXI_RIDE", 7, "correlation-1"))
		}},
		{"demand-forecast", func() (any, error) {
			return asReport(client.DemandForecast(ctx, "TAXI", "Z1", "TAXI_RIDE", time.Now(), "correlation-1"))
		}},
	}
	for _, entry := range calls {
		t.Run(entry.path, func(t *testing.T) {
			value, err := entry.call()
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(value)
			if strings.Contains(string(raw), "SECRET") {
				t.Fatal("untyped owner field escaped")
			}
			if seen["/api/v1/analytics/network/"+entry.path] != 1 {
				t.Fatal("wrong fixed owner route")
			}
		})
	}
	before := len(seen)
	if _, err = client.ProviderQuality(ctx, "TAXI", "provider/../../other", 7, "correlation-1"); err == nil {
		t.Fatal("path injection accepted")
	}
	if len(seen) != before {
		t.Fatal("invalid path reached owner")
	}
}

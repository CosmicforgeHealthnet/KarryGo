package analyticsadmin

import (
	"bytes"
	"context"
	"cosmicforge/logistics/shared/go/adminpolicy"
	"cosmicforge/logistics/shared/go/adminread"
	"cosmicforge/logistics/shared/go/serviceauth"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestScopedAnalyticsClientNeverUsesLegacyPathsOrRawCohorts(t *testing.T) {
	key := bytes.Repeat([]byte{71}, 32)
	q := adminread.Query{Scope: adminread.Scope{Market: "M1", Zone: "Z1", Organization: "org-1", Tenant: "tenant-1"}, Staff: "staff-1", Role: "staff", Purpose: "case-1", PolicyID: "test-only-policy", PolicyVersion: 1, PolicyChecksum: strings.Repeat("a", 64), Start: time.Now().UTC().Add(-time.Hour).Truncate(time.Second), End: time.Now().UTC().Truncate(time.Second), Limit: 1}
	out := Release{PolicyID: q.PolicyID, PolicyVersion: q.PolicyVersion, PolicyChecksum: q.PolicyChecksum, Scope: adminpolicy.Scope{Product: "karryam", Market: "M1", Zone: "Z1", Organization: "org-1", Tenant: "tenant-1"}, Metric: "mission-success", SourceVersion: strings.Repeat("b", 64), AsOf: time.Now().UTC(), Freshness: "LIVE", Coverage: "PARTIAL_COVERAGE", Cells: []Cell{}}
	for _, name := range []string{"completed", "failed", "cancelled", "guarantee_breaches"} {
		out.Cells = append(out.Cells, Cell{Key: name, Suppression: "PARTIAL_COVERAGE"})
	}
	for _, mode := range []string{"valid", "tenant", "raw_cohort", "value_in_partial", "duplicate", "case_alias", "missing_value", "redirect", "oversize", "denied", "rate", "unavailable"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != MissionSuccessPath || r.Method != "GET" || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
					t.Error("legacy path or browser credentials reached Analytics")
				}
				if _, err := serviceauth.NewVerifier(serviceauth.Secrets{adminread.Caller: key}, time.Minute).VerifyRequest(r); err != nil {
					t.Error(err)
				}
				if r.URL.Query().Get("tenant") != q.Scope.Tenant || r.URL.Query().Get("staff") != q.Staff || r.URL.Query().Get("authority") != "TAXI" {
					t.Error("scope/delegation/filter lost")
				}
				value := out
				value.Cells = append([]Cell{}, out.Cells...)
				switch mode {
				case "denied":
					w.WriteHeader(403)
					return
				case "rate":
					w.WriteHeader(429)
					return
				case "unavailable":
					w.WriteHeader(503)
					return
				case "redirect":
					w.Header().Set("Location", "/legacy/network/mission-success")
					w.WriteHeader(302)
					return
				case "oversize":
					w.Write(bytes.Repeat([]byte("x"), 65537))
					return
				case "tenant":
					value.Scope.Tenant = "foreign"
				case "value_in_partial":
					n := int64(99)
					value.Cells[0].Value = &n
				}
				raw, _ := json.Marshal(value)
				switch mode {
				case "raw_cohort":
					raw = bytes.Replace(raw, []byte(`"key":"completed"`), []byte(`"key":"completed","cohort_size":99`), 1)
				case "duplicate":
					raw = bytes.Replace(raw, []byte(`"metric":"mission-success"`), []byte(`"metric":"mission-success","metric":"mission-success"`), 1)
				case "case_alias":
					raw = bytes.Replace(raw, []byte(`"metric"`), []byte(`"Metric"`), 1)
				case "missing_value":
					raw = bytes.Replace(raw, []byte(`"value":null,`), nil, 1)
				}
				w.Write(raw)
			}))
			defer server.Close()
			client, err := NewClient(server.URL, key)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			result, err := client.MissionSuccess(context.Background(), q, Filters{Authority: "TAXI", ProductCode: "TAXI_PASSENGER"})
			switch mode {
			case "valid":
				if err != nil || result.Coverage != "PARTIAL_COVERAGE" {
					t.Fatal(result, err)
				}
			case "denied":
				if !errors.Is(err, ErrDenied) {
					t.Fatal(err)
				}
			case "rate":
				if !errors.Is(err, ErrRate) {
					t.Fatal(err)
				}
			case "redirect", "unavailable":
				if !errors.Is(err, ErrUnavailable) {
					t.Fatal(err)
				}
			default:
				if !errors.Is(err, ErrInvalid) {
					t.Fatal("unsafe wire contract accepted", mode, result, err)
				}
			}
		})
	}
}
func TestScopedAnalyticsClientOriginAndBoundedQuery(t *testing.T) {
	for _, origin := range []string{"file:///private", "http://user:password@localhost", "http://localhost/path", "http://localhost?override=1", "http://localhost/#fragment"} {
		if _, err := NewClient(origin, bytes.Repeat([]byte{1}, 32)); err == nil {
			t.Fatal("unsafe origin", origin)
		}
	}
	if _, err := Values(adminread.Query{}, Filters{}); err == nil {
		t.Fatal("unbounded query accepted")
	}
}

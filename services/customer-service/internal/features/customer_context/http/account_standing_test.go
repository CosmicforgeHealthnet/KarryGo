package contexthttp

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"cosmicforge/logistics/services/customer-service/internal/config"
	contextusecases "cosmicforge/logistics/services/customer-service/internal/features/customer_context/usecases"
	"cosmicforge/logistics/shared/go/serviceauth"
)

type standing struct {
	restriction string
	version     int64
	err         error
	calls       int
}

func (s *standing) AccountStanding(_ context.Context, _ string) (string, int64, error) {
	s.calls++
	return s.restriction, s.version, s.err
}

var standingNow = time.Date(2026, 10, 22, 9, 30, 15, 123456789, time.FixedZone("WAT", 3600))

const standingSecret = "dispatch-secret"

func standingRouter(t *testing.T, reader contextusecases.StandingReader) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	scopes, err := contextusecases.ParseCallerScopes("driver-dispatch-delivery-service=account_standing;driver-taxi-service=mobility_preferences")
	if err != nil {
		t.Fatal(err)
	}
	svc := contextusecases.NewService(scopes, prefs{}, locations{}).WithClock(func() time.Time { return standingNow })
	if reader != nil {
		svc = svc.WithStanding(reader)
	}
	r := gin.New()
	RegisterInternalRoutes(r.Group("/api/v1/customer"), svc, serviceauth.NewVerifier(serviceauth.Secrets{
		"driver-dispatch-delivery-service": []byte(standingSecret), "driver-taxi-service": []byte(secrets["driver-taxi-service"]),
	}, 5*time.Minute))
	return r
}

const standingPath = base + "?scopes=account_standing"

func TestAccountStandingScopeGrantedReturnsOnlyStanding(t *testing.T) {
	for _, tc := range []struct {
		restriction string
		eligible    bool
	}{{"NONE", true}, {"BOOKING_HOLD", false}, {"LOGIN_BLOCKED", false}} {
		t.Run(tc.restriction, func(t *testing.T) {
			r := standingRouter(t, &standing{restriction: tc.restriction, version: 7})
			code, body := call(t, r, "driver-dispatch-delivery-service", standingSecret, standingPath)
			if code != http.StatusOK {
				t.Fatalf("status = %d body=%v", code, body)
			}
			data := body["data"].(map[string]any)
			s, ok := data["account_standing"].(map[string]any)
			if !ok {
				t.Fatalf("account_standing missing: %v", data)
			}
			if s["booking_eligible"] != tc.eligible || s["restriction"] != tc.restriction || s["standing_version"] != float64(7) {
				t.Fatalf("standing = %v", s)
			}
			if s["as_of"] != "2026-10-22T08:30:15.123456789Z" {
				t.Fatalf("as_of = %v, want UTC RFC3339Nano read time", s["as_of"])
			}
			if data["customer_id"] != "customer-a" {
				t.Fatalf("customer_id = %v", data["customer_id"])
			}
			for _, other := range []string{"mobility_preferences", "saved_location", "guest_traveller"} {
				if _, present := data[other]; present {
					t.Fatalf("standing-only read leaked %s: %v", other, data)
				}
			}
		})
	}
}

func TestAccountStandingScopeUngrantedIsForbidden(t *testing.T) {
	r := standingRouter(t, &standing{restriction: "NONE"})
	if code, _ := call(t, r, "driver-taxi-service", secrets["driver-taxi-service"], standingPath); code != http.StatusForbidden {
		t.Fatalf("ungranted account_standing status = %d, want 403", code)
	}
}

func TestAccountStandingScopeWithoutReaderIsUnavailable(t *testing.T) {
	r := standingRouter(t, nil)
	code, body := call(t, r, "driver-dispatch-delivery-service", standingSecret, standingPath)
	if code != http.StatusServiceUnavailable {
		t.Fatalf("no reader status = %d body=%v, want 503 (never eligible)", code, body)
	}
	if data, ok := body["data"]; ok {
		t.Fatalf("unavailable standing must not answer: %v", data)
	}
}

func TestAccountStandingReadErrorIsUnavailable(t *testing.T) {
	r := standingRouter(t, &standing{err: errors.New("db down")})
	if code, _ := call(t, r, "driver-dispatch-delivery-service", standingSecret, standingPath); code != http.StatusServiceUnavailable {
		t.Fatalf("read error status = %d, want 503", code)
	}
}

func TestAccountStandingIsReadLiveEveryTime(t *testing.T) {
	reader := &standing{restriction: "BOOKING_HOLD", version: 1}
	r := standingRouter(t, reader)
	_, body := call(t, r, "driver-dispatch-delivery-service", standingSecret, standingPath)
	if body["data"].(map[string]any)["account_standing"].(map[string]any)["booking_eligible"] != false {
		t.Fatal("hold not published")
	}
	reader.restriction, reader.version = "NONE", 2
	_, body = call(t, r, "driver-dispatch-delivery-service", standingSecret, standingPath)
	if s := body["data"].(map[string]any)["account_standing"].(map[string]any); s["booking_eligible"] != true || s["standing_version"] != float64(2) {
		t.Fatalf("lifted hold not published immediately: %v", s)
	}
	if reader.calls != 2 {
		t.Fatalf("reader calls = %d, want 2", reader.calls)
	}
}

func TestExistingScopesStillCarryStandingWithAsOf(t *testing.T) {
	r := standingRouter(t, &standing{restriction: "NONE", version: 3})
	code, body := call(t, r, "driver-taxi-service", secrets["driver-taxi-service"], base+"?scopes=mobility_preferences")
	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	s, ok := body["data"].(map[string]any)["account_standing"].(map[string]any)
	if !ok || s["as_of"] == "" || s["booking_eligible"] != true {
		t.Fatalf("ride-along standing = %v", s)
	}
}

func TestDefaultCallerScopesGrantBookingAuthoritiesStanding(t *testing.T) {
	t.Setenv("CUSTOMER_CONTEXT_CALLER_SCOPES", "")
	t.Setenv("CUSTOMER_SERVICE_SECRETS", "")
	cfg := config.Load()
	scopes, err := contextusecases.ParseCallerScopes(cfg.ContextCallerScopes)
	if err != nil {
		t.Fatal(err)
	}
	trusted := serviceauth.ParseSecrets(cfg.ServiceSecrets)
	for _, caller := range []string{"driver-taxi-service", "driver-dispatch-delivery-service", "driver-hauling-service"} {
		if !scopes[caller][contextusecases.ScopeAccountStanding] {
			t.Fatalf("%s is not granted account_standing by default", caller)
		}
		if len(trusted[caller]) == 0 {
			t.Fatalf("%s has no default HMAC secret", caller)
		}
	}
	if scopes["vendor-service"][contextusecases.ScopeAccountStanding] {
		t.Fatal("vendor-service must not read account standing")
	}
}

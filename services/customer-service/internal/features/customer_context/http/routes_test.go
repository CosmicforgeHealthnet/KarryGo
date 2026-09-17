package contexthttp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	contextusecases "cosmicforge/logistics/services/customer-service/internal/features/customer_context/usecases"
	mobilitymodels "cosmicforge/logistics/services/customer-service/internal/features/mobility_context/models"
	savedlocationmodels "cosmicforge/logistics/services/customer-service/internal/features/saved_locations/models"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/serviceauth"
)

type prefs struct{}

func (prefs) GetPreferences(_ context.Context, id string) (mobilitymodels.MobilityPreferences, bool, error) {
	notes, share := "uses a cane", true
	p := mobilitymodels.DefaultPreferences(id)
	p.AccessibilityRequired, p.SpecialAssistanceNotes, p.SafetyShareDefault, p.Version = true, &notes, share, 3
	return p, true, nil
}

type locations struct{}

func (locations) GetLocation(_ context.Context, id, customerID string) (savedlocationmodels.SavedLocation, error) {
	if id != "loc-1" || customerID != "customer-a" {
		return savedlocationmodels.SavedLocation{}, apperrors.NotFound("Saved location not found.", nil)
	}
	return savedlocationmodels.SavedLocation{ID: id, CustomerID: customerID, Title: "Home", Address: "1 Road", Latitude: 6.4, Longitude: 3.4, Description: "gate code 1234"}, nil
}

var secrets = map[string]string{"driver-taxi-service": "taxi-secret", "vendor-service": "vendor-secret", "support-dispute-service": "support-secret"}

func router(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	scopes, err := contextusecases.ParseCallerScopes("driver-taxi-service=mobility_preferences|saved_location;vendor-service=saved_location")
	if err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	parsed := serviceauth.Secrets{}
	for name, secret := range secrets {
		parsed[name] = []byte(secret)
	}
	RegisterInternalRoutes(r.Group("/api/v1/customer"), contextusecases.NewService(scopes, prefs{}, locations{}), serviceauth.NewVerifier(parsed, 5*time.Minute))
	return r
}

func call(t *testing.T, r *gin.Engine, caller, secret, path string) (int, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	if caller != "" {
		if err := serviceauth.SignRequest(request, caller, []byte(secret), nil, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, request)
	var body map[string]any
	_ = json.Unmarshal(recorder.Body.Bytes(), &body)
	return recorder.Code, body
}

const base = "/api/v1/customer/internal/customers/customer-a/context"

func TestContextRejectsInvalidSignatureAndUnlistedCaller(t *testing.T) {
	r := router(t)
	if code, _ := call(t, r, "", "", base+"?scopes=mobility_preferences"); code != http.StatusUnauthorized {
		t.Fatalf("unsigned status = %d, want 401", code)
	}
	if code, _ := call(t, r, "driver-taxi-service", "wrong-secret", base+"?scopes=mobility_preferences"); code != http.StatusUnauthorized {
		t.Fatalf("bad signature status = %d, want 401", code)
	}
	if code, _ := call(t, r, "support-dispute-service", secrets["support-dispute-service"], base+"?scopes=mobility_preferences"); code != http.StatusForbidden {
		t.Fatalf("authenticated but unallowlisted caller status = %d, want 403", code)
	}
}

func TestContextIsMinimizedPerCaller(t *testing.T) {
	r := router(t)
	code, body := call(t, r, "driver-taxi-service", secrets["driver-taxi-service"], base+"?scopes=mobility_preferences")
	if code != http.StatusOK {
		t.Fatalf("taxi status = %d body=%v", code, body)
	}
	raw, _ := json.Marshal(body)
	for _, forbidden := range []string{"special_assistance_notes", "safety_share_default", "trusted", "uses a cane", "phone", "email"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("taxi context leaked %q: %s", forbidden, raw)
		}
	}
	if code, _ := call(t, r, "driver-taxi-service", secrets["driver-taxi-service"], base+"?scopes=assistance_notes"); code != http.StatusForbidden {
		t.Fatalf("ungranted scope status = %d, want 403", code)
	}
	if code, _ := call(t, r, "vendor-service", secrets["vendor-service"], base+"?scopes=mobility_preferences"); code != http.StatusForbidden {
		t.Fatalf("vendor reading mobility status = %d, want 403", code)
	}
	code, body = call(t, r, "vendor-service", secrets["vendor-service"], base+"?scopes=saved_location&saved_location_id=loc-1")
	raw, _ = json.Marshal(body)
	if code != http.StatusOK || strings.Contains(string(raw), "gate code") || strings.Contains(string(raw), "mobility_preferences") || !strings.Contains(string(raw), "context_version") {
		t.Fatalf("vendor location context = %d %s", code, raw)
	}
}

func TestContextSavedLocationIsOwnershipScoped(t *testing.T) {
	r := router(t)
	path := "/api/v1/customer/internal/customers/customer-b/context?scopes=saved_location&saved_location_id=loc-1"
	if code, _ := call(t, r, "vendor-service", secrets["vendor-service"], path); code != http.StatusNotFound {
		t.Fatalf("cross-customer location status = %d, want 404", code)
	}
}

func TestParseCallerScopesRejectsUnknownScope(t *testing.T) {
	if _, err := contextusecases.ParseCallerScopes("driver-taxi-service=trusted_contacts"); err == nil {
		t.Fatal("unknown scope must fail configuration")
	}
}

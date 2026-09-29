package contexthttp

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	contextusecases "cosmicforge/logistics/services/customer-service/internal/features/customer_context/usecases"
	mobilitymodels "cosmicforge/logistics/services/customer-service/internal/features/mobility_context/models"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/serviceauth"
)

type guests struct{}

func (guests) GetGuest(_ context.Context, customerID, id string) (mobilitymodels.GuestTraveller, error) {
	if id != "guest-1" || customerID != "customer-a" {
		return mobilitymodels.GuestTraveller{}, apperrors.NotFound("Traveller not found.", nil)
	}
	phone, label, notes := "+2348011111111", "Mother", "hard of hearing, gate code 1234"
	return mobilitymodels.GuestTraveller{ID: id, CustomerID: customerID, Name: "Ada", Phone: &phone,
		RelationshipLabel: &label, AccessibilityNotes: &notes, Active: true, Version: 4,
		MobilityRequirementsJSON: json.RawMessage(`{"wheelchair_required":true}`)}, nil
}

func guestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	scopes, err := contextusecases.ParseCallerScopes("driver-taxi-service=mobility_preferences|guest_traveller;vendor-service=saved_location")
	if err != nil {
		t.Fatal(err)
	}
	parsed := serviceauth.Secrets{}
	for name, secret := range secrets {
		parsed[name] = []byte(secret)
	}
	r := gin.New()
	service := contextusecases.NewService(scopes, prefs{}, locations{}).WithGuests(guests{})
	RegisterInternalRoutes(r.Group("/api/v1/customer"), service, serviceauth.NewVerifier(parsed, 5*time.Minute))
	return r
}

// Customer V4 §5.1/§5.2: Taxi freezes the actual traveller when a customer
// books for someone else. The context carries only what a booking snapshot
// needs, only for the owning customer, and only to a granted caller.
func TestGuestTravellerScopeIsOwnershipScopedAndMinimized(t *testing.T) {
	r := guestRouter(t)
	taxi := func(path string) (int, map[string]any) {
		return call(t, r, "driver-taxi-service", secrets["driver-taxi-service"], path)
	}

	code, body := taxi(base + "?scopes=guest_traveller&guest_traveller_id=guest-1")
	if code != http.StatusOK {
		t.Fatalf("owned traveller status = %d body=%v", code, body)
	}
	raw, _ := json.Marshal(body)
	for _, forbidden := range []string{"gate code", "hard of hearing", "Mother", "relationship", "accessibility_notes", "customer_id\":\"customer-b"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("traveller context leaked %q: %s", forbidden, raw)
		}
	}
	guest := body["data"].(map[string]any)["guest_traveller"].(map[string]any)
	if guest["name"] != "Ada" || guest["phone"] != "+2348011111111" || guest["version"].(float64) != 4 {
		t.Fatalf("traveller snapshot fields wrong: %v", guest)
	}
	if req := guest["mobility_requirements"].(map[string]any); req["wheelchair_required"] != true {
		t.Fatalf("structured requirements missing: %v", guest)
	}

	if code, _ := taxi("/api/v1/customer/internal/customers/customer-b/context?scopes=guest_traveller&guest_traveller_id=guest-1"); code != http.StatusNotFound {
		t.Fatalf("another customer's traveller status = %d, want 404", code)
	}
	if code, _ := taxi(base + "?scopes=guest_traveller"); code != http.StatusUnprocessableEntity && code != http.StatusBadRequest {
		t.Fatalf("missing traveller id status = %d, want validation", code)
	}
	if code, _ := call(t, r, "vendor-service", secrets["vendor-service"], base+"?scopes=guest_traveller&guest_traveller_id=guest-1"); code != http.StatusForbidden {
		t.Fatalf("ungranted caller status = %d, want 403", code)
	}
}

func TestGuestTravellerScopeWithoutReaderIsUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	scopes, _ := contextusecases.ParseCallerScopes("driver-taxi-service=guest_traveller")
	parsed := serviceauth.Secrets{"driver-taxi-service": []byte(secrets["driver-taxi-service"])}
	r := gin.New()
	RegisterInternalRoutes(r.Group("/api/v1/customer"), contextusecases.NewService(scopes, prefs{}, locations{}), serviceauth.NewVerifier(parsed, 5*time.Minute))
	if code, _ := call(t, r, "driver-taxi-service", secrets["driver-taxi-service"], base+"?scopes=guest_traveller&guest_traveller_id=guest-1"); code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", code)
	}
}

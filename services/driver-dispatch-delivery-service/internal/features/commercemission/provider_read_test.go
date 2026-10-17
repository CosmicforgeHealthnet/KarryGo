package commercemission

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	authhttp "cosmicforge/logistics/services/dispatch-delivery-service/internal/features/auth/http"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/httpx"
	"github.com/gin-gonic/gin"
)

type providerReadFake struct {
	missionStoreFake
	provider string
	trip     string
}

func (f *providerReadFake) ProviderMissionForTrip(_ context.Context, provider, trip string) (ProviderMissionView, error) {
	f.provider, f.trip = provider, trip
	if trip != "assigned-trip" {
		return ProviderMissionView{}, apperrors.NotFound("Assigned Commerce mission not found.", nil)
	}
	return ProviderMissionView{MissionRef: "mission", TripRef: trip, State: "PICKING_UP", Version: 3, PickupActionsAllowed: true}, nil
}
func TestProviderCommerceReadUsesAuthenticatedIdentity(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := &providerReadFake{}
	handler := NewMultiStopMissionHandler(NewMultiStopMission(MultiStopMissionOptions{Store: store}))
	engine := gin.New()
	engine.Use(httpx.ErrorHandler())
	engine.GET("/by-trip/:tripId", func(c *gin.Context) {
		c.Set(authhttp.ContextDispatchRiderID, "signed-in-provider")
		handler.readProviderTrip(c)
	})
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/by-trip/assigned-trip?provider_id=another-provider", nil))
	if response.Code != http.StatusOK || store.provider != "signed-in-provider" || store.trip != "assigned-trip" {
		t.Fatalf("scope/status: %s %s %d", store.provider, store.trip, response.Code)
	}
	var envelope map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	data := envelope["data"].(map[string]any)
	for _, key := range []string{"quote", "plan", "operation_ref", "code", "handover_code"} {
		if _, exists := data[key]; exists {
			t.Fatalf("private field exposed: %s", key)
		}
	}
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/by-trip/unassigned-trip", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("unassigned mission status: %d", response.Code)
	}
}

func TestProviderCommerceReadRejectsMalformedTripBeforeDatabase(t *testing.T) {
	_, err := (&Repository{}).ProviderMissionForTrip(context.Background(), "provider", "not-a-uuid")
	if err == nil {
		t.Fatal("malformed trip accepted")
	}
}

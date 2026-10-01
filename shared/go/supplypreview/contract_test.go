package supplypreview

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestLongestDimensionPicksLargestEdge(t *testing.T) {
	cases := []struct {
		name    string
		request Request
		want    int64
	}{
		{"length wins", Request{LengthMm: 900, WidthMm: 400, HeightMm: 300}, 900},
		{"width wins", Request{LengthMm: 400, WidthMm: 900, HeightMm: 300}, 900},
		{"height wins", Request{LengthMm: 400, WidthMm: 300, HeightMm: 900}, 900},
		{"all absent", Request{}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.request.LongestDimensionMm(); got != tc.want {
				t.Fatalf("got %d, want %d", got, tc.want)
			}
		})
	}
}

// UNKNOWN must never be encoded as a refusal about the goods. A caller that
// misreads an outage as "too heavy" stops asking forever.
func TestUnknownIsADependencyAnswerNotARefusal(t *testing.T) {
	now := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	response := Unknown("DISPATCH", now, 7)

	if response.SupplyStatus != StatusUnknown {
		t.Fatalf("status = %s, want %s", response.SupplyStatus, StatusUnknown)
	}
	if response.ReasonCode != ReasonDependencyUnavailable {
		t.Fatalf("reason = %s, want %s", response.ReasonCode, ReasonDependencyUnavailable)
	}
	if response.Serviceable {
		t.Fatal("unknown answer claimed serviceable")
	}
	if response.CapabilityVersion != 7 {
		t.Fatalf("capability version = %d, want 7", response.CapabilityVersion)
	}
	if !response.CheckedAt.Equal(now) {
		t.Fatalf("checkedAt = %v, want %v", response.CheckedAt, now)
	}
}

func TestNormalizeHandlingUppercasesAndDropsBlanks(t *testing.T) {
	got := NormalizeHandling([]string{" fragile ", "", "   ", "Chilled"})
	want := []string{"FRAGILE", "CHILLED"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

// The serialized response is a cross-service contract: no field may name a
// provider, rider, driver or vehicle, because a preview that does is an
// assignment wearing a different hat.
func TestResponseCarriesNoProviderOrVehicleIdentity(t *testing.T) {
	encoded, err := json.Marshal(Response{
		Authority:                 "HAULING",
		Serviceable:               true,
		ReasonCode:                ReasonServiceable,
		ProductCode:               "HAUL_STANDARD",
		SupplyStatus:              StatusAvailable,
		EstimatedPickupETASeconds: 900,
		CheckedAt:                 time.Now().UTC(),
		CapabilityVersion:         2,
		PolicyVersion:             "hauling_capability_v1",
	})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	lowered := strings.ToLower(string(encoded))
	for _, forbidden := range []string{
		"providerid", "provider_id", "riderid", "rider_id", "driverid", "driver_id",
		"vehicleid", "vehicle_id", "bikeid", "bike_id", "truckid", "truck_id",
		"plate", "assignee", "fleetmember", "candidate",
	} {
		if strings.Contains(lowered, forbidden) {
			t.Fatalf("response exposes %q: %s", forbidden, encoded)
		}
	}
}

// The same guard applied structurally, so a field added later cannot quietly
// reintroduce an identity leak without this test failing.
func TestResponseStructHasNoIdentityFields(t *testing.T) {
	forbidden := []string{"provider", "rider", "driver", "vehicle", "bike", "truck", "plate", "assignee", "member"}
	responseType := reflect.TypeOf(Response{})
	for i := 0; i < responseType.NumField(); i++ {
		name := strings.ToLower(responseType.Field(i).Name)
		for _, needle := range forbidden {
			if strings.Contains(name, needle) {
				t.Fatalf("Response.%s names an execution identity", responseType.Field(i).Name)
			}
		}
	}
}

type stubPreviewer struct {
	seen     Request
	response Response
	err      error
}

func (s *stubPreviewer) PreviewSupply(_ *gin.Context, request Request) (Response, error) {
	s.seen = request
	return s.response, s.err
}

func newPreviewRouter(previewer Previewer) *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterRoutes(engine.Group("/api/v1"), previewer)
	return engine
}

func TestRegisterRoutesDecodesAndAnswers(t *testing.T) {
	stub := &stubPreviewer{response: Response{
		Authority: "DISPATCH", Serviceable: true, ReasonCode: ReasonServiceable,
		SupplyStatus: StatusAvailable, CheckedAt: time.Now().UTC(),
	}}
	engine := newPreviewRouter(stub)

	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"pickupLatitude":6.5,"pickupLongitude":3.3,"weightGrams":4000}`)
	request := httptest.NewRequest(http.MethodPost, "/api/v1"+Path, body)
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if stub.seen.WeightGrams != 4000 || stub.seen.PickupLatitude != 6.5 {
		t.Fatalf("previewer saw %+v", stub.seen)
	}
}

// An empty body is a valid "can you do a plain single-pickup job here" ask.
func TestRegisterRoutesAcceptsEmptyBody(t *testing.T) {
	stub := &stubPreviewer{response: Response{Authority: "DISPATCH", SupplyStatus: StatusUnknown}}
	engine := newPreviewRouter(stub)

	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1"+Path, nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestRegisterRoutesRejectsMalformedBody(t *testing.T) {
	engine := newPreviewRouter(&stubPreviewer{})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1"+Path, strings.NewReader("{not json"))
	request.Header.Set("Content-Type", "application/json")
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

// The preview is a read. It is mounted on POST only because it carries a body,
// so no other verb may exist on the path.
func TestRegisterRoutesMountsPostOnly(t *testing.T) {
	engine := newPreviewRouter(&stubPreviewer{})
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		recorder := httptest.NewRecorder()
		engine.ServeHTTP(recorder, httptest.NewRequest(method, "/api/v1"+Path, nil))
		if recorder.Code == http.StatusOK {
			t.Fatalf("%s on preview path returned 200", method)
		}
	}
}

func TestRegisterRoutesIgnoresNilInputs(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterRoutes(nil, &stubPreviewer{})
	RegisterRoutes(engine.Group("/api/v1"), nil)
	recorder := httptest.NewRecorder()
	engine.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1"+Path, nil))
	if recorder.Code == http.StatusOK {
		t.Fatal("route was mounted with a nil previewer")
	}
}

package routing

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cosmicforge/logistics/shared/go/geo"
)

func TestClientImplementsFetcher(t *testing.T) {
	var _ Fetcher = (*Client)(nil)
}

var (
	ikeja       = geo.Point{Lat: 6.6018, Lng: 3.3515}
	victoriaIsl = geo.Point{Lat: 6.4281, Lng: 3.4219}
)

// The polyline in the canned response is the Google reference vector, so a
// successful decode proves the whole chain hands usable geometry to callers.
const referencePolyline = "_p~iF~ps|U_ulLnnqC_mqNvxq`@"

func routesServer(t *testing.T, status int, body string, capture func(*http.Request, []byte)) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		if capture != nil {
			capture(r, payload)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	return server
}

func TestRouteSuccess(t *testing.T) {
	var gotPath, gotKey, gotMask string
	var gotBody []byte

	server := routesServer(t, http.StatusOK,
		`{"routes":[{"distanceMeters":24500,"duration":"1830s","polyline":{"encodedPolyline":"`+referencePolyline+`"}}]}`,
		func(r *http.Request, body []byte) {
			gotPath = r.URL.Path
			gotKey = r.Header.Get("X-Goog-Api-Key")
			gotMask = r.Header.Get("X-Goog-FieldMask")
			gotBody = body
		})

	client := New("test-key")
	client.BaseURL = server.URL

	route, err := client.Route(context.Background(), ikeja, victoriaIsl, TravelModeDrive)
	if err != nil {
		t.Fatalf("Route returned error: %v", err)
	}

	if route.DistanceM != 24500 {
		t.Fatalf("DistanceM = %d, want 24500", route.DistanceM)
	}
	if route.DurationS != 1830 {
		t.Fatalf("DurationS = %d, want 1830", route.DurationS)
	}
	if !route.HasGeometry() {
		t.Fatal("HasGeometry() = false for a route with a polyline")
	}

	points, err := route.Points()
	if err != nil {
		t.Fatalf("Points() returned error: %v", err)
	}
	if len(points) != 3 {
		t.Fatalf("Points() returned %d points, want 3", len(points))
	}

	// Request shape: the API rejects a call with no field mask, and bills on
	// the fields requested.
	if gotPath != computeRoutesPath {
		t.Fatalf("path = %q, want %q", gotPath, computeRoutesPath)
	}
	if gotKey != "test-key" {
		t.Fatalf("X-Goog-Api-Key = %q, want %q", gotKey, "test-key")
	}
	if gotMask != fieldMask {
		t.Fatalf("X-Goog-FieldMask = %q, want %q", gotMask, fieldMask)
	}

	var sent computeRoutesRequest
	if err := json.Unmarshal(gotBody, &sent); err != nil {
		t.Fatalf("request body was not valid JSON: %v", err)
	}
	if sent.Origin.Location.LatLng.Latitude != ikeja.Lat || sent.Origin.Location.LatLng.Longitude != ikeja.Lng {
		t.Fatalf("origin = %+v, want %+v", sent.Origin.Location.LatLng, ikeja)
	}
	if sent.Destination.Location.LatLng.Latitude != victoriaIsl.Lat {
		t.Fatalf("destination = %+v, want %+v", sent.Destination.Location.LatLng, victoriaIsl)
	}
	if sent.TravelMode != string(TravelModeDrive) {
		t.Fatalf("travelMode = %q, want %q", sent.TravelMode, TravelModeDrive)
	}
}

// Bike routes must not ask for the traffic-aware preference, which the Routes
// API rejects for two-wheeler travel.
func TestRouteTwoWheelerOmitsTrafficPreference(t *testing.T) {
	var gotBody []byte
	server := routesServer(t, http.StatusOK,
		`{"routes":[{"distanceMeters":100,"duration":"60s","polyline":{"encodedPolyline":"`+referencePolyline+`"}}]}`,
		func(_ *http.Request, body []byte) { gotBody = body })

	client := New("k")
	client.BaseURL = server.URL

	if _, err := client.Route(context.Background(), ikeja, victoriaIsl, TravelModeTwoWheeler); err != nil {
		t.Fatalf("Route returned error: %v", err)
	}

	var sent computeRoutesRequest
	if err := json.Unmarshal(gotBody, &sent); err != nil {
		t.Fatalf("request body was not valid JSON: %v", err)
	}
	if sent.TravelMode != string(TravelModeTwoWheeler) {
		t.Fatalf("travelMode = %q, want %q", sent.TravelMode, TravelModeTwoWheeler)
	}
	if sent.RoutingPreference != "" {
		t.Fatalf("routingPreference = %q, want it omitted for two-wheeler routes", sent.RoutingPreference)
	}
}

func TestRouteDefaultsTravelMode(t *testing.T) {
	var gotBody []byte
	server := routesServer(t, http.StatusOK,
		`{"routes":[{"distanceMeters":1,"duration":"1s","polyline":{"encodedPolyline":"`+referencePolyline+`"}}]}`,
		func(_ *http.Request, body []byte) { gotBody = body })

	client := New("k")
	client.BaseURL = server.URL

	if _, err := client.Route(context.Background(), ikeja, victoriaIsl, ""); err != nil {
		t.Fatalf("Route returned error: %v", err)
	}
	var sent computeRoutesRequest
	_ = json.Unmarshal(gotBody, &sent)
	if sent.TravelMode != string(TravelModeDrive) {
		t.Fatalf("travelMode = %q, want the DRIVE default", sent.TravelMode)
	}
}

// A service with no key must degrade to straight-line geometry, never panic
// and never silently look configured.
func TestUnconfiguredClient(t *testing.T) {
	for _, key := range []string{"", "   "} {
		if client := New(key); client != nil {
			t.Fatalf("New(%q) = %v, want nil", key, client)
		}
	}

	var client *Client
	if client.Enabled() {
		t.Fatal("a nil client reports Enabled() = true")
	}
	if _, err := client.Route(context.Background(), ikeja, victoriaIsl, TravelModeDrive); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Route on a nil client = %v, want ErrNotConfigured", err)
	}
}

func TestRouteRejectsMissingCoordinates(t *testing.T) {
	client := New("k")
	cases := map[string][2]geo.Point{
		"null island origin":      {{}, victoriaIsl},
		"null island destination": {ikeja, {}},
		"out of range origin":     {{Lat: 200, Lng: 3}, victoriaIsl},
	}
	for name, pair := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := client.Route(context.Background(), pair[0], pair[1], TravelModeDrive); err == nil {
				t.Fatal("Route should reject a request without two real coordinates")
			}
		})
	}
}

func TestRouteSurfacesApiErrors(t *testing.T) {
	server := routesServer(t, http.StatusForbidden, `{"error":{"message":"Routes API has not been used"}}`, nil)
	client := New("k")
	client.BaseURL = server.URL

	_, err := client.Route(context.Background(), ikeja, victoriaIsl, TravelModeDrive)
	if err == nil {
		t.Fatal("Route should return an error on a non-200 response")
	}
	// The upstream message must survive, otherwise diagnosing an unenabled
	// API or an unbilled project from logs is guesswork.
	if !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "Routes API has not been used") {
		t.Fatalf("error = %v, want it to carry the status and upstream message", err)
	}
}

func TestRouteWithNoRoutesReturnsError(t *testing.T) {
	server := routesServer(t, http.StatusOK, `{"routes":[]}`, nil)
	client := New("k")
	client.BaseURL = server.URL

	if _, err := client.Route(context.Background(), ikeja, victoriaIsl, TravelModeDrive); err == nil {
		t.Fatal("Route should error when the API returns no routes")
	}
}

func TestParseDurationSeconds(t *testing.T) {
	cases := map[string]int64{
		"1830s": 1830, "0s": 0, "12.5s": 12, "": 0, "   ": 0,
		"not-a-duration": 0, "-5s": 0, "600": 600,
	}
	for in, want := range cases {
		if got := parseDurationSeconds(in); got != want {
			t.Fatalf("parseDurationSeconds(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestRouteHelpers(t *testing.T) {
	if (Route{}).HasGeometry() {
		t.Fatal("an empty route reports HasGeometry() = true")
	}
	if (Route{Polyline: "   "}).HasGeometry() {
		t.Fatal("a blank polyline reports HasGeometry() = true")
	}
	points, err := (Route{}).Points()
	if err != nil || points != nil {
		t.Fatalf("Points() on an empty route = (%v, %v), want (nil, nil)", points, err)
	}
}

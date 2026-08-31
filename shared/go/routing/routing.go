// Package routing fetches driveable road routes from the Google Routes API.
//
// It exists so the customer's map can draw the road a vehicle will actually
// take, and so trip progress can be measured along that road rather than as a
// straight line. Both booking services need it, so the integration lives here
// rather than being written twice.
//
// The client is optional by design: a service with no API key configured
// leaves the route columns empty and every consumer falls back to
// straight-line geometry. Nothing breaks, it is just less accurate.
package routing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/geo"
)

const (
	// DefaultBaseURL is the Google Routes API origin. Overridable so tests can
	// point at an httptest server.
	DefaultBaseURL = "https://routes.googleapis.com"
	// DefaultTimeout bounds one route lookup. Route fetching is never on a
	// user's critical path -- a booking is created whether or not it
	// succeeds -- so this is deliberately short.
	DefaultTimeout = 8 * time.Second

	computeRoutesPath = "/directions/v2:computeRoutes"
	// fieldMask keeps the response to the three fields that are actually
	// stored. The Routes API bills partly on returned fields, and rejects a
	// request that does not set one.
	fieldMask = "routes.duration,routes.distanceMeters,routes.polyline.encodedPolyline"
)

// TravelMode selects the road network a route is computed over.
type TravelMode string

const (
	// TravelModeDrive is used for trucks and cars.
	TravelModeDrive TravelMode = "DRIVE"
	// TravelModeTwoWheeler is used for dispatch bikes, which may legally use
	// roads a car route would avoid.
	TravelModeTwoWheeler TravelMode = "TWO_WHEELER"
)

// Route is one computed road route.
type Route struct {
	// Polyline is the road geometry in Google's encoded polyline format.
	// Decode it with geo.DecodePolyline.
	Polyline string `json:"polyline"`
	// DistanceM is the driving distance in metres, which is longer than the
	// great-circle distance between the same two points.
	DistanceM int64 `json:"distance_m"`
	// DurationS is the estimated driving time in seconds.
	DurationS int64 `json:"duration_s"`
}

// Points decodes the route geometry.
func (r Route) Points() ([]geo.Point, error) { return geo.DecodePolyline(r.Polyline) }

// HasGeometry reports whether the route carries a usable polyline.
func (r Route) HasGeometry() bool { return strings.TrimSpace(r.Polyline) != "" }

// Fetcher is the narrow interface booking services depend on, so a test can
// supply a fake and a service with no API key can supply nil.
type Fetcher interface {
	Route(ctx context.Context, origin, destination geo.Point, mode TravelMode) (Route, error)
}

// Client calls the Google Routes API.
type Client struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
}

// New builds a client, or returns nil when no API key is configured. A nil
// *Client is a valid Fetcher whose Route call reports ErrNotConfigured, so
// callers can hold one without branching.
func New(apiKey string) *Client {
	if strings.TrimSpace(apiKey) == "" {
		return nil
	}
	return &Client{
		APIKey:     strings.TrimSpace(apiKey),
		BaseURL:    DefaultBaseURL,
		HTTPClient: &http.Client{Timeout: DefaultTimeout},
	}
}

// ErrNotConfigured is returned when no API key is available.
var ErrNotConfigured = fmt.Errorf("routing: no API key configured")

// Enabled reports whether route lookups will be attempted.
func (c *Client) Enabled() bool { return c != nil && strings.TrimSpace(c.APIKey) != "" }

// Route returns the road route between two points.
func (c *Client) Route(ctx context.Context, origin, destination geo.Point, mode TravelMode) (Route, error) {
	if !c.Enabled() {
		return Route{}, ErrNotConfigured
	}
	if !origin.HasFix() || !destination.HasFix() {
		return Route{}, fmt.Errorf("routing: origin and destination must both be real coordinates")
	}
	if mode == "" {
		mode = TravelModeDrive
	}

	payload, err := json.Marshal(computeRoutesRequest{
		Origin:            waypointFor(origin),
		Destination:       waypointFor(destination),
		TravelMode:        string(mode),
		RoutingPreference: routingPreferenceFor(mode),
		PolylineEncoding:  "ENCODED_POLYLINE",
	})
	if err != nil {
		return Route{}, fmt.Errorf("routing: encode request: %w", err)
	}

	url := strings.TrimSuffix(c.baseURL(), "/") + computeRoutesPath
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return Route{}, fmt.Errorf("routing: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Goog-Api-Key", c.APIKey)
	req.Header.Set("X-Goog-FieldMask", fieldMask)

	resp, err := c.httpClient().Do(req)
	if err != nil {
		return Route{}, fmt.Errorf("routing: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return Route{}, fmt.Errorf("routing: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return Route{}, fmt.Errorf("routing: routes api returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var decoded computeRoutesResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return Route{}, fmt.Errorf("routing: decode response: %w", err)
	}
	if len(decoded.Routes) == 0 {
		return Route{}, fmt.Errorf("routing: no route between the requested points")
	}

	first := decoded.Routes[0]
	return Route{
		Polyline:  first.Polyline.EncodedPolyline,
		DistanceM: first.DistanceMeters,
		DurationS: parseDurationSeconds(first.Duration),
	}, nil
}

func (c *Client) baseURL() string {
	if c == nil || strings.TrimSpace(c.BaseURL) == "" {
		return DefaultBaseURL
	}
	return c.BaseURL
}

func (c *Client) httpClient() *http.Client {
	if c == nil || c.HTTPClient == nil {
		return &http.Client{Timeout: DefaultTimeout}
	}
	return c.HTTPClient
}

// routingPreferenceFor picks the traffic model. Two-wheeler routes do not
// support the traffic-aware preferences, so they use the default.
func routingPreferenceFor(mode TravelMode) string {
	if mode == TravelModeTwoWheeler {
		return ""
	}
	return "TRAFFIC_AWARE"
}

// parseDurationSeconds reads the protobuf duration strings the Routes API
// returns, which look like "1234s".
func parseDurationSeconds(value string) int64 {
	trimmed := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "s"))
	if trimmed == "" {
		return 0
	}
	seconds, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0
	}
	if seconds < 0 {
		return 0
	}
	return int64(seconds)
}

func waypointFor(p geo.Point) waypoint {
	return waypoint{Location: &waypointLocation{LatLng: latLng{Latitude: p.Lat, Longitude: p.Lng}}}
}

// Wire types for the Routes API.

type computeRoutesRequest struct {
	Origin            waypoint `json:"origin"`
	Destination       waypoint `json:"destination"`
	TravelMode        string   `json:"travelMode"`
	RoutingPreference string   `json:"routingPreference,omitempty"`
	PolylineEncoding  string   `json:"polylineEncoding,omitempty"`
}

type waypoint struct {
	Location *waypointLocation `json:"location,omitempty"`
}

type waypointLocation struct {
	LatLng latLng `json:"latLng"`
}

type latLng struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type computeRoutesResponse struct {
	Routes []struct {
		DistanceMeters int64  `json:"distanceMeters"`
		Duration       string `json:"duration"`
		Polyline       struct {
			EncodedPolyline string `json:"encodedPolyline"`
		} `json:"polyline"`
	} `json:"routes"`
}

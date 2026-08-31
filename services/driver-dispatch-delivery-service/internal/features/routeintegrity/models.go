package routeintegrity

import "time"

const PolicyVersion = "dispatch_route_integrity_v1"

type Point struct {
	Ref       string  `json:"ref"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}
type RoutePlan struct {
	Reference       string
	Provider        string
	EvidenceVersion string
	DistanceMeters  int64
	DurationSeconds int64
	OrderedPoints   []Point
}

type ExpectedRoute struct {
	ID                       string    `json:"route_id"`
	TripID                   string    `json:"trip_id"`
	ProviderID               string    `json:"provider_id"`
	Version                  int64     `json:"version"`
	Status                   string    `json:"status"`
	RouteReference           string    `json:"route_reference"`
	RouteProvider            string    `json:"route_provider"`
	RouteEvidenceVersion     string    `json:"route_evidence_version"`
	OrderedPoints            []Point   `json:"ordered_points"`
	DistanceMeters           int64     `json:"distance_meters"`
	DurationSeconds          int64     `json:"duration_seconds"`
	CorridorMeters           int       `json:"corridor_meters"`
	WrongWayToleranceDegrees int       `json:"wrong_way_tolerance_degrees"`
	StationarySeconds        int       `json:"stationary_seconds"`
	StaleSeconds             int       `json:"stale_seconds"`
	HighRisk                 bool      `json:"high_risk"`
	PolicyVersion            string    `json:"policy_version"`
	RerouteReason            string    `json:"reroute_reason,omitempty"`
	CreatedAt                time.Time `json:"created_at"`
}

type LocationFix struct {
	ProviderID       string    `json:"provider_id"`
	Latitude         float64   `json:"latitude"`
	Longitude        float64   `json:"longitude"`
	Heading          float64   `json:"heading"`
	Speed            float64   `json:"speed"`
	Accuracy         float64   `json:"accuracy"`
	ClientRecordedAt time.Time `json:"client_recorded_at"`
	ReceivedAt       time.Time `json:"received_at"`
}

type Alert struct {
	ID            string         `json:"alert_id"`
	TripID        string         `json:"trip_id"`
	RouteID       string         `json:"route_id"`
	RouteVersion  int64          `json:"route_version"`
	Type          string         `json:"alert_type"`
	State         string         `json:"state"`
	Severity      string         `json:"severity"`
	ObservedAt    time.Time      `json:"observed_at"`
	PolicyVersion string         `json:"policy_version"`
	Evidence      map[string]any `json:"safe_evidence"`
	DedupeKey     string         `json:"-"`
}

type Policy struct {
	Enabled                  bool
	OfflineReplayEnabled     bool
	CorridorMeters           int
	WrongWayToleranceDegrees int
	StationaryThreshold      time.Duration
	StaleThreshold           time.Duration
	HighRiskCorridorMeters   int
	HighRiskStaleThreshold   time.Duration
	MaximumAccuracyMeters    float64
	MaximumClientClockSkew   time.Duration
}

type DeviceCommand struct {
	TripID                 string         `json:"trip_id,omitempty"`
	DeviceID               string         `json:"device_id"`
	DeviceCommandID        string         `json:"device_command_id"`
	CommandType            string         `json:"command_type"`
	Payload                map[string]any `json:"payload,omitempty"`
	ClientTimestamp        time.Time      `json:"client_timestamp"`
	LastKnownServerVersion int64          `json:"last_known_server_version"`
	IdempotencyKey         string         `json:"-"`
}

type DeviceCommandResult struct {
	ID            string `json:"command_id"`
	State         string `json:"state"`
	CommandType   string `json:"command_type"`
	ServerVersion int64  `json:"server_version"`
	ReasonCode    string `json:"reason_code,omitempty"`
	Replayed      bool   `json:"replayed"`
}

type CustomerRouteStatus struct {
	TripID             string            `json:"trip_id"`
	RouteStatus        string            `json:"route_status"`
	IsStale            bool              `json:"is_stale"`
	LastUpdatedAt      *time.Time        `json:"last_updated_at,omitempty"`
	ActiveRouteVersion int64             `json:"active_route_version"`
	OpenRouteAlerts    int               `json:"open_route_alerts"`
	LastLocation       *CustomerLocation `json:"last_location,omitempty"`
}

type CustomerLocation struct {
	Latitude   float64   `json:"latitude"`
	Longitude  float64   `json:"longitude"`
	RecordedAt time.Time `json:"recorded_at"`
}

type TripRouteContext struct {
	TripID, BookingID, ProviderID, CustomerID, Status string
	SourceVersion                                     int64
	Pickup, Dropoff                                   Point
	ServiceTier                                       string
}

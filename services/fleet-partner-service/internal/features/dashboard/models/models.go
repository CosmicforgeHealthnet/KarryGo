package models

import "time"

// DashboardStats contains all aggregated counts for the dashboard.
type DashboardStats struct {
	ActiveWork     int `json:"activeWork"`
	FleetReady     int `json:"fleetReady"`
	Exceptions     int `json:"exceptions"`
	TotalProviders int `json:"totalProviders"`

	OnRoute   int `json:"onRoute"`
	Idle      int `json:"idle"`
	InTransit int `json:"inTransit"`
	Offline   int `json:"offline"`

	TotalTrips        int     `json:"totalTrips"`
	CompletedTrips    int     `json:"completedTrips"`
	OnTimePerformance float64 `json:"onTimePerformance"`
	Incidents         int     `json:"incidents"`

	ActiveDrivers int `json:"activeDrivers"`
	TripsToday    int `json:"tripsToday"`

	FleetReadyTrend    float64 `json:"fleetReadyTrend"`
	TripsTodayTrend    float64 `json:"tripsTodayTrend"`
	ExceptionsTrend    float64 `json:"exceptionsTrend"`
	ActiveDriversTrend float64 `json:"activeDriversTrend"`

	FleetReadySpark    []int `json:"fleetReadySpark"`
	TripsTodaySpark    []int `json:"tripsTodaySpark"`
	ExceptionsSpark    []int `json:"exceptionsSpark"`
	ActiveDriversSpark []int `json:"activeDriversSpark"`
}

// DashboardAlert represents a priority exception for the dashboard.
type DashboardAlert struct {
	ID          string    `json:"id"`
	AlertType   string    `json:"alertType"`
	Severity    string    `json:"severity"`
	Title       string    `json:"title"`
	Detail      string    `json:"detail"`
	Status      string    `json:"status"`
	SourceID    string    `json:"sourceId"`
	RoutePlanID string    `json:"routePlanId,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

// DashboardTrip represents active work for the dashboard.
type DashboardTrip struct {
	ID                      string   `json:"id"`
	RouteReference          string   `json:"routeReference"`
	RouteType               string   `json:"routeType"`
	ProviderID              string   `json:"providerId"`
	ProviderName            string   `json:"providerName,omitempty"`
	VehicleOrTruckReference string   `json:"vehicleOrTruckReference"`
	ExecutionService        string   `json:"executionService"`
	Status                  string   `json:"status"`
	ETA                     string   `json:"eta,omitempty"`
	Route                   string   `json:"route,omitempty"`
	RouteText               string   `json:"routeText,omitempty"`
	Branch                  string   `json:"branch,omitempty"`
	Latitude                *float64 `json:"latitude,omitempty"`
	Longitude               *float64 `json:"longitude,omitempty"`
	Speed                   *float64 `json:"speed,omitempty"`
}

// WeeklyDataPoint represents one day of chart data.
type WeeklyDataPoint struct {
	Day   string `json:"day"`
	Value int    `json:"value"`
}

// TopVehicle represents one row of the top-performing-vehicles table.
type TopVehicle struct {
	Rank    int    `json:"rank"`
	Vehicle string `json:"vehicle"`
	Trips   int    `json:"trips"`
	Status  string `json:"status"`
}

// ActivityItem represents one row of the recent-activity feed.
type ActivityItem struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	CreatedAt time.Time `json:"createdAt"`
}

// SystemStatusItem represents one row of the system-status panel.
type SystemStatusItem struct {
	Label  string `json:"label"`
	Status string `json:"status"`
}

// DashboardResponse is the complete response for GET /dashboard.
type DashboardResponse struct {
	Stats        DashboardStats     `json:"stats"`
	Alerts       []DashboardAlert   `json:"alerts"`
	ActiveTrips  []DashboardTrip    `json:"activeTrips"`
	TopVehicles  []TopVehicle       `json:"topVehicles"`
	Activity     []ActivityItem     `json:"activity"`
	SystemStatus []SystemStatusItem `json:"systemStatus"`
	WeeklyVolume []WeeklyDataPoint  `json:"weeklyVolume"`
	WeeklyTrend  []WeeklyDataPoint  `json:"weeklyTrend"`
	GeneratedAt  time.Time          `json:"generatedAt"`
}

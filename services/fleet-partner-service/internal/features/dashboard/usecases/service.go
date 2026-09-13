package usecases

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/dashboard/models"
	dispatchfeature "cosmicforge/logistics/services/fleet-partner-service/internal/features/dispatch"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
	"cosmicforge/logistics/shared/go/logging"
)

// Service aggregates dashboard data for one organization.
//
// Every section is independent: a failure in one query degrades only that
// section and never the whole response. All queries are organization-scoped.
// No external HTTP is performed; all data is read from this service's own
// database or from the in-process dispatch service.
type Service struct {
	pool     *pgxpool.Pool
	dispatch *dispatchfeature.Service
	now      func() time.Time
}

// NewService creates a new dashboard service.
func NewService(pool *pgxpool.Pool, dispatchService *dispatchfeature.Service) *Service {
	return &Service{
		pool:     pool,
		dispatch: dispatchService,
		now:      time.Now,
	}
}

// GetDashboard returns the complete dashboard payload for the caller's
// organization.
func (s *Service) GetDashboard(ctx context.Context, t tenant.Context) (*models.DashboardResponse, error) {
	if t.OrganizationID == "" {
		return nil, apperrors.BadRequest("Organization ID is required.", nil)
	}
	orgID := t.OrganizationID

	board, err := s.getDispatchBoard(ctx, t)
	if err != nil {
		logging.Error("dashboard", "dispatch board unavailable org=%s: %v", orgID, err)
		board = &dispatchfeature.Board{}
	}

	fleet := s.getFleetStats(ctx, orgID)
	trip := s.getTripStats(ctx, orgID)
	series := s.getWeeklySeries(ctx, orgID)
	top := s.getTopVehicles(ctx, orgID)
	activity := s.getRecentActivity(ctx, orgID)
	status := s.getSystemStatus(ctx)

	stats := s.buildStats(board, fleet, trip)
	stats.FleetReadyTrend, stats.FleetReadySpark = s.getTrendAndSpark(ctx, orgID, kpiFleetReady)
	stats.TripsTodayTrend, stats.TripsTodaySpark = s.getTrendAndSpark(ctx, orgID, kpiTrips)
	stats.ExceptionsTrend, stats.ExceptionsSpark = s.getTrendAndSpark(ctx, orgID, kpiExceptions)
	stats.ActiveDriversTrend, stats.ActiveDriversSpark = s.getTrendAndSpark(ctx, orgID, kpiDrivers)

	return &models.DashboardResponse{
		Stats:        stats,
		Alerts:       s.mapAlerts(board.Alerts),
		ActiveTrips:  s.mapActiveTrips(ctx, orgID, board.ActiveTrips, board.ActiveRoutes),
		TopVehicles:  top,
		Activity:     activity,
		SystemStatus: status,
		WeeklyVolume: series.volume,
		WeeklyTrend:  series.trend,
		GeneratedAt:  s.now().UTC(),
	}, nil
}

// ---------------------------------------------------------------------------
// Internal value objects
// ---------------------------------------------------------------------------

type fleetStats struct {
	fleetReady     int
	totalProviders int
	onRoute        int
	idle           int
	activeDrivers  int
}

type tripStats struct {
	tripsToday        int
	completedTrips    int
	onTimePerformance float64
}

type weeklySeries struct {
	volume []models.WeeklyDataPoint
	trend  []models.WeeklyDataPoint
}

// ---------------------------------------------------------------------------
// Dispatch board
// ---------------------------------------------------------------------------

func (s *Service) getDispatchBoard(ctx context.Context, t tenant.Context) (*dispatchfeature.Board, error) {
	timeoutCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	board, err := s.dispatch.Board(timeoutCtx, t, dispatchfeature.PageFilter{Limit: 100})
	if err != nil {
		return nil, fmt.Errorf("dispatch board: %w", err)
	}
	return &board, nil
}

// ---------------------------------------------------------------------------
// Fleet and provider counts
// ---------------------------------------------------------------------------

func (s *Service) getFleetStats(ctx context.Context, orgID string) fleetStats {
	var f fleetStats

	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM fleet_partner_assets
		WHERE organization_id = $1
		  AND operational_status = 'AVAILABLE'
		  AND lifecycle_status = 'ACTIVE'`, orgID).Scan(&f.fleetReady); err != nil {
		logging.Error("dashboard", "fleet ready count org=%s: %v", orgID, err)
	}

	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM fleet_partner_organization_provider_memberships
		WHERE organization_id = $1
		  AND status IN ('ACTIVE', 'PENDING_SETUP')`, orgID).Scan(&f.totalProviders); err != nil {
		logging.Error("dashboard", "provider count org=%s: %v", orgID, err)
	}

	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT vehicle_or_truck_reference)
		FROM fleet_partner_tracking_projections
		WHERE organization_id = $1
		  AND operational_status IN ('EN_ROUTE', 'IN_PROGRESS')
		  AND received_at > NOW() - INTERVAL '1 hour'`, orgID).Scan(&f.onRoute); err != nil {
		logging.Error("dashboard", "on route count org=%s: %v", orgID, err)
	}

	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM fleet_partner_assets a
		WHERE a.organization_id = $1
		  AND a.operational_status = 'AVAILABLE'
		  AND a.lifecycle_status = 'ACTIVE'
		  AND NOT EXISTS (
		    SELECT 1
		    FROM fleet_partner_asset_assignments aa
		    WHERE aa.organization_id = $1
		      AND aa.asset_id = a.id
		      AND aa.status = 'ACTIVE'
		  )`, orgID).Scan(&f.idle); err != nil {
		logging.Error("dashboard", "idle count org=%s: %v", orgID, err)
	}

	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM fleet_partner_workforce_profiles
		WHERE organization_id = $1
		  AND status = 'ACTIVE'
		  AND role ILIKE '%driver%'`, orgID).Scan(&f.activeDrivers); err != nil {
		logging.Error("dashboard", "active drivers count org=%s: %v", orgID, err)
	}

	return f
}

// ---------------------------------------------------------------------------
// Trip aggregates
// ---------------------------------------------------------------------------

func (s *Service) getTripStats(ctx context.Context, orgID string) tripStats {
	var t tripStats

	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM fleet_partner_trip_projections
		WHERE organization_id = $1
		  AND created_at >= date_trunc('day', NOW())`, orgID).Scan(&t.tripsToday); err != nil {
		logging.Error("dashboard", "trips today count org=%s: %v", orgID, err)
	}

	if err := s.pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM fleet_partner_trip_projections
		WHERE organization_id = $1
		  AND completed_at IS NOT NULL`, orgID).Scan(&t.completedTrips); err != nil {
		logging.Error("dashboard", "completed trips count org=%s: %v", orgID, err)
	}

	var onTime, total int
	if err := s.pool.QueryRow(ctx, `
		SELECT
		  COUNT(*) FILTER (
		    WHERE actual_arrival_at IS NOT NULL
		      AND planned_arrival_at IS NOT NULL
		      AND actual_arrival_at <= planned_arrival_at
		  ),
		  COUNT(*) FILTER (
		    WHERE actual_arrival_at IS NOT NULL
		      AND planned_arrival_at IS NOT NULL
		  )
		FROM fleet_partner_route_stops
		WHERE organization_id = $1`, orgID).Scan(&onTime, &total); err != nil {
		logging.Error("dashboard", "on time count org=%s: %v", orgID, err)
	} else if total > 0 {
		t.onTimePerformance = float64(onTime) / float64(total) * 100.0
	}

	return t
}

// ---------------------------------------------------------------------------
// Weekly series
// ---------------------------------------------------------------------------

func (s *Service) getWeeklySeries(ctx context.Context, orgID string) weeklySeries {
	out := weeklySeries{
		volume: make([]models.WeeklyDataPoint, 0, 7),
		trend:  make([]models.WeeklyDataPoint, 0, 7),
	}

	rows, err := s.pool.Query(ctx, `
		SELECT
		  to_char(d.day, 'Dy'),
		  COALESCE(v.cnt, 0),
		  COALESCE(n.cnt, 0)
		FROM generate_series(
		    date_trunc('day', NOW()) - INTERVAL '6 days',
		    date_trunc('day', NOW()),
		    INTERVAL '1 day'
		) AS d(day)
		LEFT JOIN (
		    SELECT date_trunc('day', created_at) AS day, COUNT(*) AS cnt
		    FROM fleet_partner_trip_projections
		    WHERE organization_id = $1
		      AND created_at >= NOW() - INTERVAL '7 days'
		    GROUP BY 1
		) v ON v.day = d.day
		LEFT JOIN (
		    SELECT date_trunc('day', created_at) AS day, COUNT(*) AS cnt
		    FROM fleet_partner_trip_projections
		    WHERE organization_id = $1
		      AND created_at >= NOW() - INTERVAL '7 days'
		      AND cancelled_at IS NULL
		    GROUP BY 1
		) n ON n.day = d.day
		ORDER BY d.day`, orgID)
	if err != nil {
		logging.Error("dashboard", "weekly series org=%s: %v", orgID, err)
		return out
	}
	defer rows.Close()

	for rows.Next() {
		var label string
		var v, n int
		if err := rows.Scan(&label, &v, &n); err != nil {
			logging.Error("dashboard", "weekly series scan org=%s: %v", orgID, err)
			continue
		}
		out.volume = append(out.volume, models.WeeklyDataPoint{Day: label, Value: v})
		out.trend = append(out.trend, models.WeeklyDataPoint{Day: label, Value: n})
	}
	if err := rows.Err(); err != nil {
		logging.Error("dashboard", "weekly series rows org=%s: %v", orgID, err)
	}
	return out
}

// ---------------------------------------------------------------------------
// Top vehicles
// ---------------------------------------------------------------------------

func (s *Service) getTopVehicles(ctx context.Context, orgID string) []models.TopVehicle {
	out := make([]models.TopVehicle, 0, 5)

	rows, err := s.pool.Query(ctx, `
		SELECT vehicle_or_truck_reference, COUNT(*) AS trips
		FROM fleet_partner_tracking_projections
		WHERE organization_id = $1
		  AND vehicle_or_truck_reference <> ''
		  AND received_at > NOW() - INTERVAL '7 days'
		GROUP BY vehicle_or_truck_reference
		ORDER BY trips DESC, vehicle_or_truck_reference ASC
		LIMIT 5`, orgID)
	if err != nil {
		logging.Error("dashboard", "top vehicles org=%s: %v", orgID, err)
		return out
	}
	defer rows.Close()

	rank := 0
	for rows.Next() {
		var vehicle string
		var trips int
		if err := rows.Scan(&vehicle, &trips); err != nil {
			logging.Error("dashboard", "top vehicles scan org=%s: %v", orgID, err)
			continue
		}
		rank++
		out = append(out, models.TopVehicle{
			Rank:    rank,
			Vehicle: vehicle,
			Trips:   trips,
			Status:  "Online",
		})
	}
	if err := rows.Err(); err != nil {
		logging.Error("dashboard", "top vehicles rows org=%s: %v", orgID, err)
	}
	return out
}

// ---------------------------------------------------------------------------
// Recent activity
// ---------------------------------------------------------------------------

func (s *Service) getRecentActivity(ctx context.Context, orgID string) []models.ActivityItem {
	out := make([]models.ActivityItem, 0, 10)

	rows, err := s.pool.Query(ctx, `
		SELECT id, action, resource_type, resource_id, occurred_at
		FROM fleet_partner_audit_records
		WHERE organization_id = $1
		ORDER BY occurred_at DESC
		LIMIT 10`, orgID)
	if err != nil {
		logging.Error("dashboard", "recent activity org=%s: %v", orgID, err)
		return out
	}
	defer rows.Close()

	for rows.Next() {
		var id, action, resourceType, resourceID string
		var occurredAt time.Time
		if err := rows.Scan(&id, &action, &resourceType, &resourceID, &occurredAt); err != nil {
			logging.Error("dashboard", "recent activity scan org=%s: %v", orgID, err)
			continue
		}
		out = append(out, models.ActivityItem{
			ID:        id,
			Type:      humanAction(action),
			Title:     resourceType + " " + resourceID,
			CreatedAt: occurredAt,
		})
	}
	if err := rows.Err(); err != nil {
		logging.Error("dashboard", "recent activity rows org=%s: %v", orgID, err)
	}
	return out
}

// ---------------------------------------------------------------------------
// KPI trend + sparkline
// ---------------------------------------------------------------------------

type kpiKind int

const (
	kpiFleetReady kpiKind = iota
	kpiTrips
	kpiExceptions
	kpiDrivers
)

func (s *Service) getTrendAndSpark(ctx context.Context, orgID string, kind kpiKind) (float64, []int) {
	spark := make([]int, 0, 7)
	trend := 0.0

	var thisWeek, lastWeek int
	if err := s.pool.QueryRow(ctx, trendSQL(kind), orgID).Scan(&thisWeek, &lastWeek); err != nil {
		logging.Error("dashboard", "kpi trend kind=%d org=%s: %v", kind, orgID, err)
	} else if lastWeek > 0 {
		trend = float64(thisWeek-lastWeek) / float64(lastWeek) * 100.0
	}

	rows, err := s.pool.Query(ctx, sparkSQL(kind), orgID)
	if err != nil {
		logging.Error("dashboard", "kpi spark kind=%d org=%s: %v", kind, orgID, err)
		return trend, spark
	}
	defer rows.Close()
	for rows.Next() {
		var v int
		if err := rows.Scan(&v); err != nil {
			logging.Error("dashboard", "kpi spark scan kind=%d org=%s: %v", kind, orgID, err)
			continue
		}
		spark = append(spark, v)
	}
	if err := rows.Err(); err != nil {
		logging.Error("dashboard", "kpi spark rows kind=%d org=%s: %v", kind, orgID, err)
	}
	return trend, spark
}

func trendSQL(kind kpiKind) string {
	switch kind {
	case kpiFleetReady:
		return `
			SELECT
			  COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '7 days'),
			  COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '14 days'
			                     AND created_at <  NOW() - INTERVAL '7 days')
			FROM fleet_partner_assets
			WHERE organization_id = $1
			  AND lifecycle_status = 'ACTIVE'`
	case kpiTrips:
		return `
			SELECT
			  COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '7 days'),
			  COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '14 days'
			                     AND created_at <  NOW() - INTERVAL '7 days')
			FROM fleet_partner_trip_projections
			WHERE organization_id = $1`
	case kpiExceptions:
		return `
			SELECT
			  COUNT(*) FILTER (WHERE occurred_at >= NOW() - INTERVAL '7 days'),
			  COUNT(*) FILTER (WHERE occurred_at >= NOW() - INTERVAL '14 days'
			                     AND occurred_at <  NOW() - INTERVAL '7 days')
			FROM fleet_partner_audit_records
			WHERE organization_id = $1
			  AND action LIKE '%alert%'`
	case kpiDrivers:
		return `
			SELECT
			  COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '7 days'),
			  COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '14 days'
			                     AND created_at <  NOW() - INTERVAL '7 days')
			FROM fleet_partner_workforce_profiles
			WHERE organization_id = $1
			  AND status = 'ACTIVE'
			  AND role ILIKE '%driver%'`
	}
	return ""
}

func sparkSQL(kind kpiKind) string {
	switch kind {
	case kpiFleetReady:
		return `
			SELECT COUNT(a.id)
			FROM generate_series(
			    date_trunc('day', NOW()) - INTERVAL '6 days',
			    date_trunc('day', NOW()),
			    INTERVAL '1 day'
			) AS d(day)
			LEFT JOIN fleet_partner_assets a
			  ON a.organization_id = $1
			 AND a.lifecycle_status = 'ACTIVE'
			 AND a.created_at >= d.day
			 AND a.created_at <  d.day + INTERVAL '1 day'
			GROUP BY d.day
			ORDER BY d.day`
	case kpiTrips:
		return `
			SELECT COUNT(t.id)
			FROM generate_series(
			    date_trunc('day', NOW()) - INTERVAL '6 days',
			    date_trunc('day', NOW()),
			    INTERVAL '1 day'
			) AS d(day)
			LEFT JOIN fleet_partner_trip_projections t
			  ON t.organization_id = $1
			 AND t.created_at >= d.day
			 AND t.created_at <  d.day + INTERVAL '1 day'
			GROUP BY d.day
			ORDER BY d.day`
	case kpiExceptions:
		return `
			SELECT COUNT(r.id)
			FROM generate_series(
			    date_trunc('day', NOW()) - INTERVAL '6 days',
			    date_trunc('day', NOW()),
			    INTERVAL '1 day'
			) AS d(day)
			LEFT JOIN fleet_partner_audit_records r
			  ON r.organization_id = $1
			 AND r.action LIKE '%alert%'
			 AND r.occurred_at >= d.day
			 AND r.occurred_at <  d.day + INTERVAL '1 day'
			GROUP BY d.day
			ORDER BY d.day`
	case kpiDrivers:
		return `
			SELECT COUNT(p.id)
			FROM generate_series(
			    date_trunc('day', NOW()) - INTERVAL '6 days',
			    date_trunc('day', NOW()),
			    INTERVAL '1 day'
			) AS d(day)
			LEFT JOIN fleet_partner_workforce_profiles p
			  ON p.organization_id = $1
			 AND p.status = 'ACTIVE'
			 AND p.role ILIKE '%driver%'
			 AND p.created_at >= d.day
			 AND p.created_at <  d.day + INTERVAL '1 day'
			GROUP BY d.day
			ORDER BY d.day`
	}
	return ""
}

// ---------------------------------------------------------------------------
// System status
// ---------------------------------------------------------------------------

func (s *Service) getSystemStatus(ctx context.Context) []models.SystemStatusItem {
	db := "Online"
	if err := s.pool.Ping(ctx); err != nil {
		db = "Offline"
	}
	return []models.SystemStatusItem{
		{Label: "Database", Status: db},
		{Label: "Dispatch Service", Status: "Online"},
		{Label: "Tracking Ingestion", Status: "Online"},
		{Label: "Audit Log", Status: "Online"},
	}
}

// ---------------------------------------------------------------------------
// Builders
// ---------------------------------------------------------------------------

func (s *Service) buildStats(board *dispatchfeature.Board, f fleetStats, t tripStats) models.DashboardStats {
	activeWork := len(board.ActiveTrips)
	if activeWork == 0 && len(board.ActiveRoutes) > 0 {
		activeWork = len(board.ActiveRoutes)
	}

	exceptions := 0
	for _, a := range board.Alerts {
		if a.Status == "OPEN" || a.Status == "ACKNOWLEDGED" {
			exceptions++
		}
	}

	return models.DashboardStats{
		ActiveWork:        activeWork,
		FleetReady:        f.fleetReady,
		Exceptions:        exceptions,
		TotalProviders:    f.totalProviders,
		OnRoute:           f.onRoute,
		Idle:              f.idle,
		InTransit:         maxInt(activeWork-f.onRoute, 0),
		Offline:           maxInt(f.fleetReady-activeWork, 0),
		TotalTrips:        activeWork,
		CompletedTrips:    t.completedTrips,
		OnTimePerformance: t.onTimePerformance,
		Incidents:         exceptions,
		ActiveDrivers:     f.activeDrivers,
		TripsToday:        t.tripsToday,
	}
}

func (s *Service) mapAlerts(alerts []dispatchfeature.OperationalAlert) []models.DashboardAlert {
	if len(alerts) == 0 {
		return []models.DashboardAlert{}
	}
	out := make([]models.DashboardAlert, 0, len(alerts))
	for _, a := range alerts {
		if a.Status == "RESOLVED" || a.Status == "DISMISSED" {
			continue
		}
		out = append(out, models.DashboardAlert{
			ID:          a.ID,
			AlertType:   a.AlertType,
			Severity:    a.Severity,
			Title:       alertTitle(a),
			Detail:      alertDetail(a),
			Status:      a.Status,
			SourceID:    a.SourceID,
			RoutePlanID: a.RoutePlanID,
			CreatedAt:   a.CreatedAt,
		})
	}
	return out
}

func alertTitle(a dispatchfeature.OperationalAlert) string {
	switch a.AlertType {
	case "SAFETY_BLOCK":
		return fmt.Sprintf("Vehicle %s safety block", a.SourceID)
	case "TRIP_SLA":
		return fmt.Sprintf("Trip %s behind SLA", a.SourceID)
	case "DOCUMENT_EXPIRY":
		return "Provider documents expiring"
	case "MAINTENANCE_DUE":
		return "Maintenance due"
	case "NO_ELIGIBLE_PROVIDER":
		return "No eligible provider available"
	default:
		return a.AlertType
	}
}

func alertDetail(a dispatchfeature.OperationalAlert) string {
	switch a.AlertType {
	case "SAFETY_BLOCK":
		return "Inspection failed"
	case "TRIP_SLA":
		return "SLA breach"
	case "DOCUMENT_EXPIRY":
		return "Documents require renewal"
	case "MAINTENANCE_DUE":
		return "Vehicles require maintenance"
	default:
		return a.AlertType
	}
}

func (s *Service) mapActiveTrips(
	ctx context.Context,
	orgID string,
	trips []dispatchfeature.TrackingProjection,
	routes []dispatchfeature.RoutePlan,
) []models.DashboardTrip {
	if len(trips) == 0 {
		return []models.DashboardTrip{}
	}

	routeMap := make(map[string]dispatchfeature.RoutePlan, len(routes))
	for _, r := range routes {
		routeMap[r.ID] = r
	}

	out := make([]models.DashboardTrip, 0, len(trips))
	for _, trip := range trips {
		if trip.OperationalStatus == "COMPLETED" || trip.OperationalStatus == "CANCELLED" {
			continue
		}

		dt := models.DashboardTrip{
			ID:                      trip.CanonicalTripReference,
			RouteReference:          trip.CanonicalTripReference,
			RouteType:               "DELIVERY",
			ProviderID:              trip.ProviderID,
			VehicleOrTruckReference: trip.VehicleOrTruckReference,
			ExecutionService:        trip.SourceService,
			Status:                  trip.OperationalStatus,
			ETA:                     s.calculateETA(trip),
			Route:                   trip.CanonicalTripReference,
			Latitude:                trip.Latitude,
			Longitude:               trip.Longitude,
			Speed:                   trip.Speed,
		}

		if route, ok := routeMap[trip.RoutePlanID]; ok {
			dt.RouteReference = route.RouteReference
			dt.RouteType = route.RouteType
			dt.Branch = route.BranchID
			dt.RouteText = s.buildRouteText(ctx, orgID, route.ID)
		}

		out = append(out, dt)
	}
	return out
}

func (s *Service) buildRouteText(ctx context.Context, orgID, routeID string) string {
	var first, last string

	rows, err := s.pool.Query(ctx, `
		SELECT COALESCE(location_reference, '')
		FROM fleet_partner_route_stops
		WHERE organization_id = $1
		  AND route_plan_id = $2
		ORDER BY sequence ASC
		LIMIT 1`, orgID, routeID)
	if err != nil {
		return ""
	}
	if rows.Next() {
		_ = rows.Scan(&first)
	}
	rows.Close()

	rows2, err := s.pool.Query(ctx, `
		SELECT COALESCE(location_reference, '')
		FROM fleet_partner_route_stops
		WHERE organization_id = $1
		  AND route_plan_id = $2
		ORDER BY sequence DESC
		LIMIT 1`, orgID, routeID)
	if err != nil {
		return ""
	}
	if rows2.Next() {
		_ = rows2.Scan(&last)
	}
	rows2.Close()

	if first == "" && last == "" {
		return ""
	}
	if first == last {
		return first
	}
	return first + " → " + last
}

func (s *Service) calculateETA(trip dispatchfeature.TrackingProjection) string {
	if trip.ReceivedAt.IsZero() {
		return "Pending"
	}
	switch trip.OperationalStatus {
	case "COMPLETED":
		return ""
	case "IN_PROGRESS", "EN_ROUTE":
		elapsed := time.Since(trip.ReceivedAt)
		switch {
		case elapsed < 5*time.Minute:
			return "~5 min"
		case elapsed < 15*time.Minute:
			return "~15 min"
		case elapsed < 30*time.Minute:
			return "~30 min"
		default:
			return ">30 min"
		}
	case "ASSIGNED":
		return "Pending"
	}
	return ""
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// humanAction turns "asset.created.v1" into "Asset created".
func humanAction(action string) string {
	action = strings.TrimSuffix(action, ".v1")
	action = strings.ReplaceAll(action, ".", " ")
	action = strings.ReplaceAll(action, "_", " ")
	action = strings.TrimSpace(action)
	if action == "" {
		return ""
	}
	return strings.ToUpper(action[:1]) + action[1:]
}
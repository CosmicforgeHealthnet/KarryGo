package usecases

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/features/dispatch/models"
	routemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/routes/models"
	trackingmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/tracking/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/errorcodes"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

func (s *Service) Board(ctx context.Context, t tenant.Context, f models.PageFilter) (models.Board, error) {
	if e := s.gate(s.o.Flags.ControlTower, false, "control_tower_disabled"); e != nil {
		return models.Board{}, e
	}
	if f.BranchID != "" && !t.BranchPermitted(f.BranchID) {
		return models.Board{}, nf()
	}
	if s.o.Routes == nil || s.o.Tracking == nil {
		return models.Board{}, errorcodes.Unavailable("control_tower_unavailable", "Control tower projections are unavailable.")
	}
	routes, e := s.o.Routes.BoardRoutes(ctx, t.OrganizationID, f)
	if e != nil {
		return models.Board{}, e
	}
	tracking, e := s.o.Tracking.BoardProjections(ctx, t.OrganizationID, f)
	if e != nil {
		return models.Board{}, e
	}
	alerts, _, e := s.ListAlerts(ctx, t, f)
	if e != nil {
		return models.Board{}, e
	}
	unassigned, e := s.unassigned(ctx, t, f)
	if e != nil {
		return models.Board{}, e
	}
	pending, e := s.assignmentSummary(ctx, t, f, "PENDING_PROVIDER_RESPONSE")
	if e != nil {
		return models.Board{}, e
	}
	accepted, e := s.assignmentSummary(ctx, t, f, "ACCEPTED,READY_FOR_EXECUTION")
	if e != nil {
		return models.Board{}, e
	}
	summary := map[string]int{trackingmodels.Fresh: 0, trackingmodels.Aging: 0, trackingmodels.Stale: 0, trackingmodels.Unknown: 0, trackingmodels.Gapped: 0}
	gaps := false
	// Freshness is recomputed at read time and written back into each trip:
	// the stored status is only as current as the last maintenance sweep, and
	// a stale fix must never be presented as live.
	for i := range tracking {
		state := trackingmodels.Freshness(tracking[i].SourceTimestamp, s.o.Now(), s.o.TrackingFreshAfter, s.o.TrackingStaleAfter, tracking[i].GapStatus != "CURRENT")
		tracking[i].FreshnessStatus = state
		summary[state]++
		if state == trackingmodels.Gapped {
			gaps = true
		}
	}
	active := []routemodels.RoutePlan{}
	for _, r := range routes {
		if r.Status == routemodels.RoutePublished || r.Status == routemodels.RouteInProgress {
			active = append(active, r)
		}
	}
	providers, vehicles, projectionGaps, e := s.boardAuthoritySummaries(ctx, t, f)
	if e != nil {
		return models.Board{}, e
	}
	capacity := map[string]int{
		"unassignedWork":      len(unassigned),
		"pendingAssignments":  len(pending),
		"acceptedAssignments": len(accepted),
		"activeRoutes":        len(active),
		"activeTrips":         len(tracking),
	}
	return models.Board{Unassigned: unassigned, PendingAssignments: pending, AcceptedAssignments: accepted, ActiveRoutes: active, ActiveTrips: tracking, Alerts: alerts, TrackingSummary: summary, CapacitySummary: capacity, ProviderAvailability: providers, VehicleAvailability: vehicles, ProjectionGaps: projectionGaps, ProjectionIncomplete: summary[trackingmodels.Stale]+summary[trackingmodels.Unknown]+summary[trackingmodels.Gapped] > 0, ReconciliationRequired: gaps}, nil
}

// boardAuthoritySummaries uses persisted candidate and tracking projections only;
// it never fans out to provider or vehicle authorities while rendering the board.
func (s *Service) boardAuthoritySummaries(ctx context.Context, t tenant.Context, f models.PageFilter) ([]map[string]any, []map[string]any, []map[string]any, error) {
	candidates, e := s.repo.ActiveCandidates(ctx, t.OrganizationID, f.BranchID)
	if e != nil {
		return nil, nil, nil, e
	}
	providers, vehicles := []map[string]any{}, []map[string]any{}
	seenProviders, seenVehicles := map[string]bool{}, map[string]bool{}
	for _, c := range candidates {
		if !t.BranchPermitted(c.BranchID) {
			continue
		}
		if c.ProviderID != "" && !seenProviders[c.ProviderID] {
			providers = append(providers, map[string]any{"providerId": c.ProviderID, "availability": c.Eligibility, "source": "candidate_projection", "generatedAt": c.GeneratedAt, "expiresAt": c.ExpiresAt})
			seenProviders[c.ProviderID] = true
		}
		if c.VehicleReference != "" && !seenVehicles[c.VehicleReference] {
			vehicles = append(vehicles, map[string]any{"vehicleOrTruckReference": c.VehicleReference, "availability": c.Eligibility, "source": "candidate_projection", "generatedAt": c.GeneratedAt, "expiresAt": c.ExpiresAt})
			seenVehicles[c.VehicleReference] = true
		}
	}
	projectionGaps, e := s.o.Tracking.ProjectionGaps(ctx, t.OrganizationID)
	if e != nil {
		return nil, nil, nil, e
	}
	return providers, vehicles, projectionGaps, nil
}
func (s *Service) ControlTower(ctx context.Context, t tenant.Context, f models.PageFilter) (models.Board, error) {
	return s.Board(ctx, t, f)
}
func (s *Service) Unassigned(ctx context.Context, t tenant.Context, f models.PageFilter) ([]map[string]any, error) {
	if e := s.gate(s.o.Flags.DispatchPlanning, false, "dispatch_planning_disabled"); e != nil {
		return nil, e
	}
	return s.unassigned(ctx, t, f)
}
func (s *Service) unassigned(ctx context.Context, t tenant.Context, f models.PageFilter) ([]map[string]any, error) {
	work, e := s.repo.UnassignedWork(ctx, t.OrganizationID, f.BranchID)
	if e != nil {
		return nil, e
	}
	out := []map[string]any{}
	for _, w := range work {
		if b, _ := w["branchId"].(string); t.BranchPermitted(b) {
			out = append(out, w)
		}
	}
	return out, nil
}
func (s *Service) assignmentSummary(ctx context.Context, t tenant.Context, f models.PageFilter, statuses string) ([]map[string]any, error) {
	rows, e := s.repo.AssignmentsByStatus(ctx, t.OrganizationID, statuses)
	if e != nil {
		return nil, e
	}
	out := []map[string]any{}
	for _, a := range rows {
		if !t.BranchPermitted(a.BranchID) || f.BranchID != "" && f.BranchID != a.BranchID || f.Service != "" && f.Service != a.ExecutionService || f.ProviderID != "" && f.ProviderID != a.ProviderID || f.VehicleReference != "" && f.VehicleReference != a.VehicleReference {
			continue
		}
		out = append(out, map[string]any{"id": a.ID, "branchId": a.BranchID, "workOrigin": a.WorkOrigin, "status": a.Status, "providerId": a.ProviderID, "vehicleOrTruckReference": a.VehicleReference, "executionService": a.ExecutionService, "responseDeadline": a.ResponseDeadline})
	}
	return out, nil
}

func (s *Service) CreateGeofence(ctx context.Context, t tenant.Context, in models.CreateGeofenceInput) (models.Geofence, error) {
	if x := s.gate(s.o.Flags.Geofences, true, "geofences_disabled"); x != nil {
		return models.Geofence{}, x
	}
	if in.BranchID != "" && !t.BranchPermitted(in.BranchID) {
		return models.Geofence{}, nf()
	}
	if strings.TrimSpace(in.Name) == "" || in.IdempotencyKey == "" {
		return models.Geofence{}, apperrors.BadRequest("name and Idempotency-Key are required.", nil)
	}
	if x := models.ValidateGeometry(in.GeometryType, in.Geometry); x != nil {
		return models.Geofence{}, apperrors.BadRequest(x.Error(), x)
	}
	in.EventRules = models.NormalizeRules(in.EventRules)
	for _, r := range in.EventRules {
		if !models.ValidEventRule(r) {
			return models.Geofence{}, apperrors.BadRequest("eventRules contains an invalid rule.", nil)
		}
	}
	fp := models.Fingerprint(in)
	stored := in
	stored.GeometryType = strings.ToUpper(in.GeometryType)
	var out models.Geofence
	x := s.tx(ctx, func(tx pgx.Tx) error {
		var e error
		if out, e = s.repo.CreateGeofence(ctx, tx, t.OrganizationID, stored, t.ActorID, fp); e != nil {
			return e
		}
		return s.record(ctx, tx, t, "geofence.created.v1", "geofence", out.ID, out.Version, out)
	})
	return out, x
}
func (s *Service) GetGeofence(ctx context.Context, t tenant.Context, id string) (models.Geofence, error) {
	if x := s.gate(s.o.Flags.Geofences, false, "geofences_disabled"); x != nil {
		return models.Geofence{}, x
	}
	g, e := s.repo.GetGeofence(ctx, nil, t.OrganizationID, id, false)
	if e != nil || !t.BranchPermitted(g.BranchID) {
		return models.Geofence{}, nf()
	}
	return g, nil
}

// ListGeofences returns one page of geofences in the caller's branch scope
// and the cursor of the next page ("" on the last page).
func (s *Service) ListGeofences(ctx context.Context, t tenant.Context, f models.PageFilter) ([]models.Geofence, string, error) {
	if x := s.gate(s.o.Flags.Geofences, false, "geofences_disabled"); x != nil {
		return nil, "", x
	}
	size := models.BoundedLimit(f.Limit)
	items, e := s.repo.ListGeofences(ctx, t.OrganizationID, f, size+1)
	if e != nil {
		return nil, "", e
	}
	items, next := models.TrimPage(items, size, func(v models.Geofence) (time.Time, string) { return v.CreatedAt, v.ID })
	out := []models.Geofence{}
	for _, g := range items {
		if t.BranchPermitted(g.BranchID) {
			out = append(out, g)
		}
	}
	return out, next, nil
}
func (s *Service) UpdateGeofence(ctx context.Context, t tenant.Context, id string, in models.CreateGeofenceInput, expected int64) (models.Geofence, error) {
	if x := s.gate(s.o.Flags.Geofences, true, "geofences_disabled"); x != nil {
		return models.Geofence{}, x
	}
	if x := models.RequireVersion(expected); x != nil {
		return models.Geofence{}, x
	}
	if x := models.ValidateGeometry(in.GeometryType, in.Geometry); x != nil {
		return models.Geofence{}, apperrors.BadRequest(x.Error(), x)
	}
	in.GeometryType = strings.ToUpper(in.GeometryType)
	in.EventRules = models.NormalizeRules(in.EventRules)
	var g models.Geofence
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		g, x = s.repo.UpdateGeofence(ctx, tx, t.OrganizationID, id, expected, in)
		if x == models.ErrNotFound {
			return models.ErrConflict
		}
		if x != nil {
			return x
		}
		if !t.BranchPermitted(g.BranchID) {
			return nf()
		}
		return s.record(ctx, tx, t, "geofence.updated.v1", "geofence", g.ID, g.Version, g)
	})
	if e == models.ErrNotFound {
		return g, models.ErrConflict
	}
	if !t.BranchPermitted(g.BranchID) {
		return models.Geofence{}, nf()
	}
	return g, e
}
func (s *Service) SetGeofenceActive(ctx context.Context, t tenant.Context, id, status string, expected int64) (models.Geofence, error) {
	if x := s.gate(s.o.Flags.Geofences, true, "geofences_disabled"); x != nil {
		return models.Geofence{}, x
	}
	if x := models.RequireVersion(expected); x != nil {
		return models.Geofence{}, x
	}
	var g models.Geofence
	e := s.tx(ctx, func(tx pgx.Tx) error {
		var x error
		g, x = s.repo.SetGeofenceStatus(ctx, tx, t.OrganizationID, id, expected, status)
		if x == models.ErrNotFound {
			return models.ErrConflict
		}
		if x != nil {
			return x
		}
		if !t.BranchPermitted(g.BranchID) {
			return nf()
		}
		event := "geofence.deactivated.v1"
		if status == "ACTIVE" {
			event = "geofence.activated.v1"
		}
		return s.record(ctx, tx, t, event, "geofence", g.ID, g.Version, g)
	})
	if e == models.ErrNotFound {
		return g, models.ErrConflict
	}
	if !t.BranchPermitted(g.BranchID) {
		return models.Geofence{}, nf()
	}
	return g, e
}

// ListGeofenceEvents returns one page of geofence events in the caller's
// branch scope and the cursor of the next page ("" on the last page).
func (s *Service) ListGeofenceEvents(ctx context.Context, t tenant.Context, f models.GeofenceEventFilter) ([]models.GeofenceEvent, string, error) {
	if x := s.gate(s.o.Flags.Geofences, false, "geofences_disabled"); x != nil {
		return nil, "", x
	}
	if f.BranchID != "" && !t.BranchPermitted(f.BranchID) {
		return []models.GeofenceEvent{}, "", nil
	}
	size := models.BoundedLimit(f.Limit)
	items, e := s.repo.ListGeofenceEvents(ctx, t.OrganizationID, f, size+1)
	if e != nil {
		return nil, "", e
	}
	items, next := models.TrimPage(items, size, func(v models.GeofenceEvent) (time.Time, string) { return v.OccurredAt, v.ID })
	now := s.o.Now()
	out := []models.GeofenceEvent{}
	for _, v := range items {
		if !t.BranchPermitted(v.BranchID) {
			continue
		}
		v.DataFreshness = trackingmodels.Freshness(v.OccurredAt, now, s.o.TrackingFreshAfter, s.o.TrackingStaleAfter, false)
		v.AgeSeconds = int64(now.Sub(v.OccurredAt).Seconds())
		out = append(out, v)
	}
	return out, next, nil
}

func (s *Service) createAlert(ctx context.Context, tx pgx.Tx, t tenant.Context, req models.AlertRequest) (models.OperationalAlert, error) {
	req.SLADueAt = s.o.Now().Add(s.o.AlertSLA)
	a, inserted, e := s.repo.UpsertAlert(ctx, tx, t.OrganizationID, req)
	if e == nil && inserted {
		e = s.record(ctx, tx, t, "operational_alert.created.v1", "operational_alert", a.ID, a.Version, map[string]any{"alert": a, "notificationIntent": true})
		if e == nil && s.o.Flags.Workers {
			_, e = s.o.Queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: t.OrganizationID, JobType: models.JobAlertEscalate, DedupeKey: "alert-sla:" + a.ID, Payload: map[string]any{"organizationId": t.OrganizationID, "alertId": a.ID}, ScheduledAt: a.SLADueAt, MaxAttempts: 8})
		}
	}
	return a, e
}

// RaiseAlert raises an operational alert inside the caller's transaction,
// with the Section 16 dedupe, audit, outbox and escalation rules. Tracking
// raises gap alerts through it.
func (s *Service) RaiseAlert(ctx context.Context, tx pgx.Tx, t tenant.Context, req models.AlertRequest) error {
	_, e := s.createAlert(ctx, tx, t, req)
	return e
}

// CreateSystemAlert is the narrow cross-feature port used by asset safety.
// It preserves the Section 16 alert dedupe, audit, outbox and escalation rules.
func (s *Service) CreateSystemAlert(ctx context.Context, organizationID, kind, sourceID, reference, severity, message string) error {
	if x := s.gate(s.o.Flags.OperationalAlerts, true, "operational_alerts_disabled"); x != nil {
		return x
	}
	t := tenant.Context{OrganizationID: organizationID, ActorType: tenant.ActorTypeSystem, ActorID: "asset-safety", CorrelationID: reference}
	return s.tx(ctx, func(tx pgx.Tx) error {
		_, err := s.createAlert(ctx, tx, t, models.AlertRequest{Kind: kind, Severity: severity, SourceType: "ASSET", SourceID: sourceID, AssignmentID: reference, Condition: "asset-safety:" + kind + ":" + sourceID + ":" + reference})
		return err
	})
}

// MarketplaceAssignmentExpired is the narrow cross-feature port the
// marketplace assignment expiry worker uses (FLEET-11.11c). It raises the same
// ASSIGNMENT_EXPIRED alert the private route-stop expiry raises, inside the
// caller's expiry transaction, through the shared Section 16 path (dedupe,
// audit, operational_alert.created.v1 with its notification intent, SLA
// escalation).
//
// It is idempotent per assignment for the life of the alert, not only while
// it is open: a replayed expiry finds the alert it already raised, even if a
// dispatcher has since resolved it, and raises nothing new. When operational
// alerts are switched off it does nothing rather than failing, because an
// alert sink being disabled must never block releasing an unresponsive
// provider.
func (s *Service) MarketplaceAssignmentExpired(ctx context.Context, tx pgx.Tx, organizationID, branchID, assignmentID string) error {
	if s.o.Flags.KillSwitch || !s.o.Flags.OperationalAlerts {
		return nil
	}
	condition := "marketplace-assignment-expired:" + assignmentID
	exists, e := s.repo.AlertConditionRaised(ctx, tx, organizationID, condition)
	if e != nil {
		return e
	}
	if exists {
		return nil
	}
	t := tenant.Context{OrganizationID: organizationID, ActorType: tenant.ActorTypeSystem, ActorID: "assignment-expiry", CorrelationID: assignmentID}
	_, e = s.createAlert(ctx, tx, t, models.AlertRequest{Kind: "ASSIGNMENT_EXPIRED", Severity: "HIGH", SourceType: "MARKETPLACE_ASSIGNMENT", SourceID: assignmentID, BranchID: branchID, AssignmentID: assignmentID, Condition: condition})
	return e
}
func (s *Service) GetAlert(ctx context.Context, t tenant.Context, id string) (models.OperationalAlert, error) {
	if x := s.gate(s.o.Flags.OperationalAlerts, false, "operational_alerts_disabled"); x != nil {
		return models.OperationalAlert{}, x
	}
	a, e := s.repo.GetAlert(ctx, nil, t.OrganizationID, id, false)
	if e != nil || !t.BranchPermitted(a.BranchID) {
		return models.OperationalAlert{}, nf()
	}
	return a, nil
}

// ListAlerts returns one page of alerts in the caller's branch scope and the
// cursor of the next page ("" on the last page).
func (s *Service) ListAlerts(ctx context.Context, t tenant.Context, f models.PageFilter) ([]models.OperationalAlert, string, error) {
	if x := s.gate(s.o.Flags.OperationalAlerts, false, "operational_alerts_disabled"); x != nil {
		return nil, "", x
	}
	size := models.BoundedLimit(f.Limit)
	items, e := s.repo.ListAlerts(ctx, t.OrganizationID, f, size+1)
	if e != nil {
		return nil, "", e
	}
	items, next := models.TrimPage(items, size, func(v models.OperationalAlert) (time.Time, string) { return v.CreatedAt, v.ID })
	out := []models.OperationalAlert{}
	for _, a := range items {
		if t.BranchPermitted(a.BranchID) {
			out = append(out, a)
		}
	}
	return out, next, nil
}
func (s *Service) AcknowledgeAlert(ctx context.Context, t tenant.Context, id string, expected int64) (models.OperationalAlert, error) {
	return s.updateAlert(ctx, t, id, expected, models.AlertAcknowledged, "", "")
}
func (s *Service) ResolveAlert(ctx context.Context, t tenant.Context, id string, expected int64, code, reason string) (models.OperationalAlert, error) {
	if strings.TrimSpace(code) == "" || strings.TrimSpace(reason) == "" {
		return models.OperationalAlert{}, apperrors.BadRequest("resolutionCode and resolutionReason are required.", nil)
	}
	return s.updateAlert(ctx, t, id, expected, models.AlertResolved, code, reason)
}
func (s *Service) updateAlert(ctx context.Context, t tenant.Context, id string, expected int64, to, code, reason string) (models.OperationalAlert, error) {
	if x := models.RequireVersion(expected); x != nil {
		return models.OperationalAlert{}, x
	}
	var out models.OperationalAlert
	e := s.tx(ctx, func(tx pgx.Tx) error {
		old, x := s.repo.GetAlert(ctx, tx, t.OrganizationID, id, true)
		if x != nil || !t.BranchPermitted(old.BranchID) {
			return nf()
		}
		if !models.ValidAlertTransition(old.Status, to) {
			return errors.New("invalid alert transition")
		}
		out, x = s.repo.TransitionAlert(ctx, tx, t.OrganizationID, id, expected, to, t.OrganizationUserID, code, reason)
		if x != nil {
			return models.ErrConflict
		}
		if x = s.repo.AlertHistory(ctx, tx, t.OrganizationID, id, old.Status, to, t.ActorID, reason, out.Version); x != nil {
			return x
		}
		return s.record(ctx, tx, t, "operational_alert."+strings.ToLower(to)+".v1", "operational_alert", id, out.Version, out)
	})
	return out, e
}

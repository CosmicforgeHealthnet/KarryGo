// Package usecases owns the Section 16 tracking-projection workflows:
// idempotent, version-ordered ingestion of authoritative Dispatch/Hauling
// trip events, gap detection and reconciliation, branch-scoped reads and the
// freshness/retention sweep. Alerts and route visibility are reached through
// the ports declared here.
package usecases

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	dispatchmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/dispatch/models"
	routemodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/routes/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/tracking/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/jobs"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platform/tenant"
	"cosmicforge/logistics/shared/go/apperrors"
)

// Repository is the tracking persistence port. Methods taking a
// database.Querier join the caller's transaction.
type Repository interface {
	RecordInbound(ctx context.Context, q database.Querier, e models.TrackingEvent, payloadHash string) (bool, error)
	LockProjection(ctx context.Context, q database.Querier, sourceService, trip string) (int64, string, string, error)
	RecordConflict(ctx context.Context, q database.Querier, e models.TrackingEvent, existingHash, receivedHash string) error
	UpsertProjection(ctx context.Context, q database.Querier, e models.TrackingEvent, receivedAt time.Time, payloadHash, freshness, gapStatus string, reconciledAt *time.Time, retentionExpiresAt time.Time) error
	Get(ctx context.Context, org, trip string) (models.TrackingProjection, error)
	List(ctx context.Context, org string, f dispatchmodels.PageFilter, limit int) ([]models.TrackingProjection, error)
	ProjectionGaps(ctx context.Context, org string) ([]map[string]any, error)
	GapStatus(ctx context.Context, org, sourceService, trip string) (string, error)
	ResolveConflicts(ctx context.Context, org, sourceService, trip string) error
	MarkConflictsReconciling(ctx context.Context, org, sourceService, trip string) (int64, error)
	SweepProjections(ctx context.Context, q database.Querier, freshAfter, staleAfter time.Duration) error
	StaleProjections(ctx context.Context, q database.Querier) ([]models.StaleProjection, error)
}

// EventPublisher appends domain events to the outbox inside a transaction.
type EventPublisher interface {
	Publish(context.Context, pgx.Tx, string, string, string, int64, string, string, any) error
}

// AlertRaiser raises a Section 16 operational alert inside the caller's
// transaction.
type AlertRaiser interface {
	RaiseAlert(ctx context.Context, tx pgx.Tx, t tenant.Context, req dispatchmodels.AlertRequest) error
}

// RouteScope resolves the branch of a route plan.
type RouteScope interface {
	RouteBranch(ctx context.Context, org, routeID string) (string, error)
}

// Options wires the service. Repository is required.
type Options struct {
	Pool                                                      *pgxpool.Pool
	Repository                                                Repository
	Events                                                    EventPublisher
	Alerts                                                    AlertRaiser
	Routes                                                    RouteScope
	Queue                                                     *jobs.Queue
	Flags                                                     dispatchmodels.Flags
	TrackingFreshAfter, TrackingStaleAfter, TrackingRetention time.Duration
	Now                                                       func() time.Time
}

// Service implements the tracking-projection workflows.
type Service struct {
	o    Options
	repo Repository
}

// NewService builds the service.
func NewService(o Options) *Service {
	if o.Queue == nil {
		o.Queue = jobs.NewQueue(o.Pool)
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.TrackingFreshAfter <= 0 {
		o.TrackingFreshAfter = 2 * time.Minute
	}
	if o.TrackingStaleAfter <= 0 {
		o.TrackingStaleAfter = 10 * time.Minute
	}
	if o.TrackingRetention <= 0 {
		o.TrackingRetention = 24 * time.Hour
	}
	return &Service{o: o, repo: o.Repository}
}

func (s *Service) freshness(v models.TrackingProjection) string {
	return models.Freshness(v.SourceTimestamp, s.o.Now(), s.o.TrackingFreshAfter, s.o.TrackingStaleAfter, v.GapStatus != "CURRENT")
}

func sha256sum(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }
func nullableReconciledAt(reconciled bool, at time.Time) *time.Time {
	if !reconciled {
		return nil
	}
	return &at
}

func (s *Service) IngestTracking(ctx context.Context, e models.TrackingEvent) (models.TrackingProjection, string, error) {
	if x := s.o.Flags.Gate(s.o.Flags.TrackingProjections, true, "tracking_projections_disabled"); x != nil {
		return models.TrackingProjection{}, "", x
	}
	if !s.o.Flags.TrackingSourceConfigured {
		return models.TrackingProjection{}, "", errors.New("authoritative tracking source is not configured")
	}
	if e.OrganizationID == "" || e.CanonicalTripReference == "" || e.EventID == "" || e.SourceVersion <= 0 || e.SourceTimestamp.IsZero() {
		return models.TrackingProjection{}, "", apperrors.BadRequest("tracking event identity, version and sourceTimestamp are required.", nil)
	}
	if e.SourceService != "driver-dispatch-delivery-service" && e.SourceService != "driver-hauling-service" {
		return models.TrackingProjection{}, "", apperrors.BadRequest("sourceService is not authoritative.", nil)
	}
	if (e.Latitude == nil) != (e.Longitude == nil) {
		return models.TrackingProjection{}, "", apperrors.BadRequest("latitude and longitude must be supplied together.", nil)
	}
	raw, _ := json.Marshal(e)
	sum := sha256sum(raw)
	now := s.o.Now().UTC()
	var out models.TrackingProjection
	disposition := "APPLIED"
	err := database.WithinTx(ctx, s.o.Pool, func(tx pgx.Tx) error {
		inserted, x := s.repo.RecordInbound(ctx, tx, e, sum)
		if x != nil {
			return x
		}
		if !inserted {
			disposition = "DUPLICATE"
			return nil
		}
		oldVersion, oldHash, oldGapStatus, x := s.repo.LockProjection(ctx, tx, e.SourceService, e.CanonicalTripReference)
		if x != nil && !errors.Is(x, dispatchmodels.ErrNotFound) {
			return x
		}
		if x == nil && e.SourceVersion < oldVersion {
			disposition = "LOWER_VERSION_IGNORED"
			return nil
		}
		if x == nil && e.SourceVersion == oldVersion {
			if oldHash == sum {
				disposition = "DUPLICATE"
				return nil
			}
			disposition = "CONFLICT"
			return s.repo.RecordConflict(ctx, tx, e, oldHash, sum)
		}
		gap := x == nil && (oldGapStatus != "CURRENT" || e.SourceVersion > oldVersion+1)
		reconciled := gap && e.ReconcilesThroughVersion >= e.SourceVersion
		if reconciled {
			gap = false
			disposition = "RECONCILED"
		}
		fresh := models.Freshness(e.SourceTimestamp, now, s.o.TrackingFreshAfter, s.o.TrackingStaleAfter, gap)
		gapState := "CURRENT"
		if gap {
			gapState = "GAP_DETECTED"
			disposition = "GAP_DETECTED"
		}
		if x = s.repo.UpsertProjection(ctx, tx, e, now, sum, fresh, gapState, nullableReconciledAt(reconciled, now), now.Add(s.o.TrackingRetention)); x != nil {
			return x
		}
		out = models.TrackingProjection{OrganizationID: e.OrganizationID, SourceService: e.SourceService, CanonicalTripReference: e.CanonicalTripReference, AssignmentID: e.AssignmentID, RoutePlanID: e.RoutePlanID, ProviderID: e.ProviderID, VehicleOrTruckReference: e.VehicleOrTruckReference, Latitude: e.Latitude, Longitude: e.Longitude, Heading: e.Heading, Speed: e.Speed, Accuracy: e.Accuracy, OperationalStatus: e.OperationalStatus, SourceTimestamp: e.SourceTimestamp, ReceivedAt: now, SourceVersion: e.SourceVersion, FreshnessStatus: fresh, GapStatus: gapState, LastReconciledAt: nullableReconciledAt(reconciled, now)}
		if gap {
			_, x = s.o.Queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: e.OrganizationID, JobType: models.JobTrackingReconcile, DedupeKey: e.SourceService + ":" + e.CanonicalTripReference, Payload: map[string]any{"organizationId": e.OrganizationID, "sourceService": e.SourceService, "canonicalTripReference": e.CanonicalTripReference}, MaxAttempts: 8})
			if x != nil {
				return x
			}
			if s.o.Flags.OperationalAlerts && s.o.Alerts != nil {
				x = s.o.Alerts.RaiseAlert(ctx, tx, tenant.Context{OrganizationID: e.OrganizationID, ActorType: tenant.ActorTypeSystem, ActorID: "tracking-projection"}, dispatchmodels.AlertRequest{Kind: "TRACKING_GAP", Severity: "HIGH", SourceType: "CANONICAL_TRIP", SourceID: e.CanonicalTripReference, RoutePlanID: e.RoutePlanID, AssignmentID: e.AssignmentID, CanonicalTripReference: e.CanonicalTripReference, Condition: "tracking-gap:" + e.SourceService + ":" + e.CanonicalTripReference})
				if x != nil {
					return x
				}
			}
		}
		if s.o.Flags.Geofences {
			_, x = s.o.Queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: e.OrganizationID, JobType: dispatchmodels.JobGeofenceEvaluate, DedupeKey: e.SourceService + ":" + e.CanonicalTripReference + ":" + fmt.Sprint(e.SourceVersion), Payload: map[string]any{"organizationId": e.OrganizationID, "sourceService": e.SourceService, "canonicalTripReference": e.CanonicalTripReference, "routePlanId": e.RoutePlanID}, MaxAttempts: 8})
			if x != nil {
				return x
			}
		}
		if e.RoutePlanID != "" {
			_, x = s.o.Queue.Enqueue(ctx, tx, jobs.EnqueueRequest{OrganizationID: e.OrganizationID, JobType: routemodels.JobRouteProgress, DedupeKey: e.RoutePlanID + ":" + fmt.Sprint(e.SourceVersion), Payload: map[string]any{"organizationId": e.OrganizationID, "routePlanId": e.RoutePlanID}, MaxAttempts: 8})
			if x != nil {
				return x
			}
		}
		if s.o.Events != nil {
			eventType := "tracking_projection.updated.v1"
			if gap {
				eventType = "tracking_projection.gap_detected.v1"
			} else if reconciled {
				eventType = "tracking_projection.reconciled.v1"
			}
			if x = s.o.Events.Publish(ctx, tx, eventType, "tracking_projection", e.CanonicalTripReference, e.SourceVersion, e.OrganizationID, "", map[string]any{"canonicalTripReference": e.CanonicalTripReference, "sourceService": e.SourceService, "sourceVersion": e.SourceVersion, "freshnessStatus": fresh, "gapStatus": gapState}); x != nil {
				return x
			}
		}
		return nil
	})
	return out, disposition, err
}

// visible reports whether a route-linked projection is inside the caller's
// branch scope.
func (s *Service) visible(ctx context.Context, t tenant.Context, v models.TrackingProjection) bool {
	if v.RoutePlanID == "" {
		return true
	}
	if s.o.Routes == nil {
		return false
	}
	branch, x := s.o.Routes.RouteBranch(ctx, t.OrganizationID, v.RoutePlanID)
	return x == nil && t.BranchPermitted(branch)
}

func (s *Service) GetTracking(ctx context.Context, t tenant.Context, trip string) (models.TrackingProjection, error) {
	if x := s.o.Flags.Gate(s.o.Flags.TrackingProjections, false, "tracking_projections_disabled"); x != nil {
		return models.TrackingProjection{}, x
	}
	v, e := s.repo.Get(ctx, t.OrganizationID, trip)
	if e != nil {
		return models.TrackingProjection{}, dispatchmodels.NotFound()
	}
	if !s.visible(ctx, t, v) {
		return models.TrackingProjection{}, dispatchmodels.NotFound()
	}
	v.FreshnessStatus = s.freshness(v)
	return v, nil
}

// ListTracking returns one page of projections in the caller's branch scope
// and the cursor of the next page ("" on the last page).
func (s *Service) ListTracking(ctx context.Context, t tenant.Context, f dispatchmodels.PageFilter) ([]models.TrackingProjection, string, error) {
	if x := s.o.Flags.Gate(s.o.Flags.TrackingProjections, false, "tracking_projections_disabled"); x != nil {
		return nil, "", x
	}
	size := dispatchmodels.BoundedLimit(f.Limit)
	items, e := s.repo.List(ctx, t.OrganizationID, f, size+1)
	items, next := dispatchmodels.TrimPage(items, size, func(v models.TrackingProjection) (time.Time, string) {
		return v.ReceivedAt, models.ProjectionKey(v.SourceService, v.CanonicalTripReference)
	})
	out := items[:0]
	for i := range items {
		items[i].FreshnessStatus = s.freshness(items[i])
		if !s.visible(ctx, t, items[i]) {
			continue
		}
		out = append(out, items[i])
	}
	return out, next, e
}

// BoardProjections reads projections for the control-tower board without the
// capability gate or route scoping the board applies itself.
func (s *Service) BoardProjections(ctx context.Context, org string, f dispatchmodels.PageFilter) ([]models.TrackingProjection, error) {
	return s.repo.List(ctx, org, f, dispatchmodels.BoundedLimit(f.Limit))
}

// ProjectionGaps reads the organization's projections with a sequence gap.
func (s *Service) ProjectionGaps(ctx context.Context, org string) ([]map[string]any, error) {
	return s.repo.ProjectionGaps(ctx, org)
}

// Projection reads one projection for a worker, without scoping.
func (s *Service) Projection(ctx context.Context, org, trip string) (models.TrackingProjection, error) {
	return s.repo.Get(ctx, org, trip)
}

// SweepProjections deletes expired projections, recomputes stored freshness
// and returns the projections now STALE, inside the caller's transaction.
func (s *Service) SweepProjections(ctx context.Context, tx pgx.Tx, staleNeeded bool) ([]models.StaleProjection, error) {
	if e := s.repo.SweepProjections(ctx, tx, s.o.TrackingFreshAfter, s.o.TrackingStaleAfter); e != nil {
		return nil, e
	}
	if !staleNeeded {
		return nil, nil
	}
	return s.repo.StaleProjections(ctx, tx)
}

// RegisterWorkers binds the tracking reconciliation job.
func (s *Service) RegisterWorkers(w *jobs.Worker) {
	w.Register(models.JobTrackingReconcile, s.handleTrackingReconcile)
}

func (s *Service) handleTrackingReconcile(ctx context.Context, job jobs.Job) error {
	if s.o.Flags.KillSwitch || !s.o.Flags.Workers {
		return errors.New("section16 workers disabled")
	}
	var p struct {
		OrganizationID         string `json:"organizationId"`
		SourceService          string `json:"sourceService"`
		CanonicalTripReference string `json:"canonicalTripReference"`
	}
	if e := json.Unmarshal(job.Payload, &p); e != nil {
		return e
	}
	if p.OrganizationID != job.OrganizationID {
		return jobs.ErrTenantMismatch
	}
	if !s.o.Flags.TrackingSourceConfigured {
		return errors.New("authoritative tracking read-back unavailable")
	}
	gapStatus, e := s.repo.GapStatus(ctx, p.OrganizationID, p.SourceService, p.CanonicalTripReference)
	if e != nil {
		return e
	}
	if gapStatus == "CURRENT" {
		return s.repo.ResolveConflicts(ctx, p.OrganizationID, p.SourceService, p.CanonicalTripReference)
	}
	n, e := s.repo.MarkConflictsReconciling(ctx, p.OrganizationID, p.SourceService, p.CanonicalTripReference)
	if e != nil {
		return e
	}
	if n == 0 {
		return errors.New("authoritative read-back required; no safe reconciliation result available")
	}
	return errors.New("authoritative read-back required; outcome not fabricated")
}

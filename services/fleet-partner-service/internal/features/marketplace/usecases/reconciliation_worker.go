package usecases

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"cosmicforge/logistics/services/fleet-partner-service/internal/database"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/models"
	"cosmicforge/logistics/services/fleet-partner-service/internal/features/marketplace/repositories"
	"cosmicforge/logistics/services/fleet-partner-service/internal/platformclients"
)

// ReconciliationConfig tunes the projection reconciliation worker.
type ReconciliationConfig struct {
	Owner       string
	BatchSize   int
	MaxAttempts int
	BaseBackoff time.Duration
	MaxBackoff  time.Duration
	// StaleAfter is how long a projection may go without a source event
	// before the sweep marks it STALE.
	StaleAfter time.Duration
	// SweepLimit bounds how many rows one stale sweep touches.
	SweepLimit int
	// Lease is how long a claimed case stays owned before recovery returns it
	// to the queue. It must exceed the time one read-back can take, or a slow
	// source lets a second worker claim a case still being processed.
	Lease time.Duration
}

func (c ReconciliationConfig) withDefaults() ReconciliationConfig {
	if c.Owner == "" {
		c.Owner = "projection-reconciliation-worker"
	}
	if c.BatchSize <= 0 {
		c.BatchSize = 20
	}
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 8
	}
	if c.BaseBackoff <= 0 {
		c.BaseBackoff = 10 * time.Second
	}
	if c.MaxBackoff <= 0 {
		c.MaxBackoff = 15 * time.Minute
	}
	if c.StaleAfter <= 0 {
		c.StaleAfter = 15 * time.Minute
	}
	if c.SweepLimit <= 0 {
		c.SweepLimit = 500
	}
	if c.Lease <= 0 {
		c.Lease = 2 * time.Minute
	}
	return c
}

func (c ReconciliationConfig) backoffFor(attempt int) time.Duration {
	delay := c.BaseBackoff
	for i := 0; i < attempt && delay < c.MaxBackoff; i++ {
		delay *= 2
	}
	if delay > c.MaxBackoff {
		delay = c.MaxBackoff
	}
	return delay
}

// ReconciliationResult summarises one worker pass.
type ReconciliationResult struct {
	Claimed        int
	Resolved       int
	Deferred       int
	Exhausted      int
	StaleRequests  int64
	StaleTrips     int64
	SourceUnknown  int
	NotReconciled  int
	SourceNotFound int
	// Recovered counts cases returned to the queue after a worker abandoned
	// them mid-flight.
	Recovered int64
	// CasesOpened counts repairs enqueued by the sweep rather than by an
	// ingest-time gap.
	CasesOpened int
}

// RunReconciliation performs one reconciliation pass and one stale sweep.
//
// Ordering matters: cases are repaired first, then the sweep runs. A sweep
// that ran first would mark projections STALE that this very pass is about to
// bring current, producing an alarming freshness state that is already false
// by the time anyone reads it.
func (s *Service) RunReconciliation(ctx context.Context,
	config ReconciliationConfig) (ReconciliationResult, error) {
	config = config.withDefaults()
	var result ReconciliationResult

	// Recover first. A case abandoned by a crashed worker is still IN_PROGRESS
	// with an expired lease, and until it is returned to the queue this pass
	// would skip straight past it.
	recovered, err := s.recoverReconciliationLeases(ctx)
	if err != nil {
		return result, err
	}
	result.Recovered = recovered

	// Open cases for projections that went stale, gapped or unknown without
	// any event arriving to notice. This is what makes reconciliation reach
	// normal production trips rather than only those an ingest error flagged.
	opened, err := s.openTripReconciliationCases(ctx, config)
	if err != nil {
		return result, err
	}
	result.CasesOpened = opened

	claimed, err := s.claimReconciliationCases(ctx, config)
	if err != nil {
		return result, err
	}
	result.Claimed = len(claimed)

	for index := range claimed {
		if ctx.Err() != nil {
			// Unprocessed cases keep their pushed-out next_attempt_at and are
			// picked up by the next pass; nothing is lost by stopping here.
			return result, nil
		}
		outcome, err := s.reconcileCase(ctx, claimed[index], config)
		if err != nil {
			return result, err
		}
		switch outcome {
		case reconcileResolved:
			result.Resolved++
		case reconcileExhausted:
			result.Exhausted++
		case reconcileSourceUnknown:
			result.SourceUnknown++
			result.Deferred++
		case reconcileSourceNotFound:
			result.SourceNotFound++
			result.Deferred++
		default:
			result.Deferred++
		}
	}

	if result.StaleRequests, err = s.opts.Projections.MarkStale(ctx,
		config.StaleAfter, config.SweepLimit); err != nil {
		return result, err
	}
	if result.StaleTrips, err = s.opts.Trips.MarkStaleTrips(ctx,
		config.StaleAfter, config.SweepLimit); err != nil {
		return result, err
	}
	return result, nil
}

// claimReconciliationCases leases a batch.
//
// The claim marks each case IN_PROGRESS and pushes next_attempt_at forward in
// the same transaction as the SELECT ... FOR UPDATE SKIP LOCKED. Without that
// second step the row lock would be released at commit and a second worker
// could immediately claim the same case: SKIP LOCKED only excludes concurrent
// transactions, not later ones.
func (s *Service) claimReconciliationCases(ctx context.Context,
	config ReconciliationConfig) ([]repositories.ReconciliationCase, error) {
	var claimed []repositories.ReconciliationCase
	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		var claimErr error
		claimed, claimErr = s.opts.Trips.ClaimReconciliationCases(ctx, tx, config.BatchSize)
		if claimErr != nil {
			return claimErr
		}
		for _, item := range claimed {
			// A lease, not a resolution. ResolveReconciliation increments
			// attempts, so claiming through it charged every case twice per
			// pass — once here, once when deferring — halving the retry budget
			// without anyone noticing.
			if err := s.opts.Trips.ClaimTripReconciliationLease(ctx, tx, item.ID,
				config.Owner, config.Lease); err != nil {
				return err
			}
		}
		return nil
	})
	return claimed, err
}

// recoverReconciliationLeases returns abandoned cases to the queue.
func (s *Service) recoverReconciliationLeases(ctx context.Context) (int64, error) {
	var recovered int64
	err := database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		var recoverErr error
		recovered, recoverErr = s.opts.Trips.RecoverExpiredReconciliationLeases(ctx, tx)
		return recoverErr
	})
	return recovered, err
}

// openTripReconciliationCases enqueues repairs for untrusted trip projections.
//
// Without this the only cases in the queue are those an ingested event
// happened to notice. A projection that simply stopped receiving events —
// the failure mode reconciliation exists for — would never be examined.
func (s *Service) openTripReconciliationCases(ctx context.Context,
	config ReconciliationConfig) (int, error) {
	trips, err := s.opts.Trips.ListForReconciliation(ctx, config.SweepLimit)
	if err != nil {
		return 0, err
	}
	if len(trips) == 0 {
		return 0, nil
	}

	opened := 0
	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		for index := range trips {
			trip := trips[index]
			// OpenReconciliation absorbs the conflict against the live-case
			// unique index, so a projection already queued is skipped rather
			// than duplicated.
			if openErr := s.opts.Trips.OpenReconciliation(ctx, tx, trip.OrganizationID,
				trip.SourceService, models.ReconciliationAggregateTrip, trip.PlatformTripID,
				"projection_"+strings.ToLower(trip.FreshnessState),
				time.Now().UTC()); openErr != nil {
				return openErr
			}
			opened++
		}
		return nil
	})
	return opened, err
}

// reconcileOutcome classifies one case's result.
type reconcileOutcome string

const (
	reconcileResolved       reconcileOutcome = "RESOLVED"
	reconcileDeferred       reconcileOutcome = "DEFERRED"
	reconcileExhausted      reconcileOutcome = "EXHAUSTED"
	reconcileSourceUnknown  reconcileOutcome = "SOURCE_UNKNOWN"
	reconcileSourceNotFound reconcileOutcome = "SOURCE_NOT_FOUND"
)

// reconcileCase repairs one aggregate from authoritative source state.
func (s *Service) reconcileCase(ctx context.Context, item repositories.ReconciliationCase,
	config ReconciliationConfig) (reconcileOutcome, error) {
	client, ok := s.opts.Executions.For(platformclients.CategoryDispatch)
	if !ok || client == nil || !client.Configured() {
		// Fail closed. An unconfigured source client cannot answer, and
		// guessing the aggregate's state is precisely what reconciliation
		// exists to avoid, so the case waits for configuration to be fixed.
		// This does not consume the attempt budget, because nothing about the
		// aggregate was actually tried.
		return reconcileDeferred, s.deferCase(ctx, item, config,
			"source_client_not_configured")
	}

	// Scoped to this case so a retry is the same logical read rather than a
	// new one, and so two cases never share an idempotency key.
	meta := platformclients.CallMeta{IdempotencyKey: "reconcile:" + item.ID}

	switch item.AggregateType {
	case models.ReconciliationAggregateRequest:
		return s.reconcileRequest(ctx, item, config, meta, client)
	case models.ReconciliationAggregateTrip:
		return s.reconcileTrip(ctx, item, config, meta, client)
	default:
		// A case we have no procedure for will never resolve by retrying.
		// Close it as failed so it stops consuming batch slots and shows up
		// as an unhandled aggregate rather than an endless retry.
		return reconcileExhausted, database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
			return s.opts.Trips.ResolveReconciliation(ctx, tx, item.ID,
				models.ReconciliationFailed, "unsupported_aggregate_type", nil)
		})
	}
}

// reconcileRequest repairs a request projection.
func (s *Service) reconcileRequest(ctx context.Context, item repositories.ReconciliationCase,
	config ReconciliationConfig, meta platformclients.CallMeta,
	client platformclients.ExecutionContract) (reconcileOutcome, error) {
	view, err := client.ReadRequest(ctx, meta, item.AggregateSourceID)
	if err != nil {
		return s.classifyReadFailure(ctx, item, config, err)
	}

	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		projection, findErr := s.opts.Projections.FindBySourceRequestAnyOrganization(ctx, tx,
			item.SourceService, item.AggregateSourceID)
		if findErr != nil {
			return findErr
		}
		if err := s.opts.Projections.ReconcileFromSource(ctx, tx, projection.ID,
			view.SourceVersion, view.Status,
			projectionStatusForSourceRequest(view.Status)); err != nil {
			return err
		}
		if err := s.opts.Trips.ResolveGaps(ctx, tx, item.SourceService,
			models.ReconciliationAggregateRequest, item.AggregateSourceID); err != nil {
			return err
		}
		return s.opts.Trips.ResolveReconciliation(ctx, tx, item.ID,
			models.ReconciliationResolved, "", nil)
	})
	if errors.Is(err, repositories.ErrNotFound) {
		// The source knows this request but fleet holds no projection for it.
		// There is nothing to repair and no amount of retrying will create
		// one, so the case is closed rather than left to churn.
		return reconcileExhausted, database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
			return s.opts.Trips.ResolveReconciliation(ctx, tx, item.ID,
				models.ReconciliationFailed, "projection_not_found", nil)
		})
	}
	if err != nil {
		return reconcileDeferred, err
	}
	return reconcileResolved, nil
}

// reconcileTrip repairs a trip projection.
func (s *Service) reconcileTrip(ctx context.Context, item repositories.ReconciliationCase,
	config ReconciliationConfig, meta platformclients.CallMeta,
	client platformclients.ExecutionContract) (reconcileOutcome, error) {
	view, err := client.ReadTrip(ctx, meta, item.AggregateSourceID)
	if err != nil {
		return s.classifyReadFailure(ctx, item, config, err)
	}

	err = database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		trip, findErr := s.opts.Trips.GetBySourceTripAnyOrganization(ctx, tx,
			item.SourceService, item.AggregateSourceID)
		if findErr != nil {
			return findErr
		}
		// ReconcileTripFromSource only ever moves the version forward, so a
		// lagging read replica cannot drag the projection backwards and
		// re-open the gap it was called to close.
		if err := s.opts.Trips.ReconcileTripFromSource(ctx, tx, trip.ID,
			view.SourceVersion, view.Status, view.CompletedAt, view.CancelledAt); err != nil {
			return err
		}
		// The request projection derives its status from the trip's outcome.
		// Repairing the trip without it would leave the organization's request
		// view contradicting the trip view it was just told to trust.
		if err := s.repairAssociatedRequest(ctx, tx, trip, view); err != nil {
			return err
		}
		if err := s.opts.Trips.ResolveGaps(ctx, tx, item.SourceService,
			models.ReconciliationAggregateTrip, item.AggregateSourceID); err != nil {
			return err
		}
		return s.opts.Trips.ResolveReconciliation(ctx, tx, item.ID,
			models.ReconciliationResolved, "", nil)
	})
	if errors.Is(err, repositories.ErrNotFound) {
		return reconcileExhausted, database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
			return s.opts.Trips.ResolveReconciliation(ctx, tx, item.ID,
				models.ReconciliationFailed, "trip_projection_not_found", nil)
		})
	}
	if err != nil {
		return reconcileDeferred, err
	}
	return reconcileResolved, nil
}

// repairAssociatedRequest brings the request projection in line with an
// authoritative trip outcome.
//
// Only terminal trip states propagate. A trip still running says nothing about
// where fleet's own request workflow has reached, and overwriting the request
// status from mid-flight trip state would undo local progress.
func (s *Service) repairAssociatedRequest(ctx context.Context, tx pgx.Tx,
	trip *models.TripProjection, view platformclients.SourceTripView) error {
	if trip.PlatformRequestID == nil || *trip.PlatformRequestID == "" {
		return nil
	}
	status := ""
	switch view.Status {
	case "completed":
		status = models.ProjectionCompleted
	case "cancelled":
		status = models.ProjectionCancelled
	default:
		return nil
	}

	projection, err := s.opts.Projections.FindBySourceRequestAnyOrganization(ctx, tx,
		trip.SourceService, *trip.PlatformRequestID)
	if errors.Is(err, repositories.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.opts.Projections.SetProjectionStatusUnversioned(ctx, tx, projection.ID, status)
}

// classifyReadFailure decides whether a failed read-back should be retried.
//
// Every failure here defers rather than resolving. That is the fail-closed
// rule this worker exists to uphold: an unreachable, erroring or ambiguous
// source tells us nothing about the aggregate, and writing a guessed state
// would replace a projection we know to be uncertain with one that merely
// looks certain.
func (s *Service) classifyReadFailure(ctx context.Context, item repositories.ReconciliationCase,
	config ReconciliationConfig, readErr error) (reconcileOutcome, error) {
	code := "source_read_failed"
	if errors.Is(readErr, platformclients.ErrNotConfigured) {
		code = "source_client_not_configured"
	}
	// A read that failed for any reason — outage, timeout, or the source
	// reporting no such aggregate — is treated identically: retry, then give
	// up and leave the case for an operator. In particular a "not found" is
	// never taken as licence to terminate the projection. A read replica that
	// has not caught up looks exactly like a deleted aggregate, and one of
	// those two readings is destructive and irreversible.
	if item.Attempts+1 >= config.MaxAttempts {
		return reconcileExhausted, database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
			return s.opts.Trips.ResolveReconciliation(ctx, tx, item.ID,
				models.ReconciliationFailed, code, nil)
		})
	}
	return reconcileSourceUnknown, s.deferCase(ctx, item, config, code)
}

// deferCase returns a case to the queue with backoff.
func (s *Service) deferCase(ctx context.Context, item repositories.ReconciliationCase,
	config ReconciliationConfig, code string) error {
	next := time.Now().UTC().Add(config.backoffFor(item.Attempts))
	return database.WithinTx(ctx, s.opts.Pool, func(tx pgx.Tx) error {
		return s.opts.Trips.ResolveReconciliation(ctx, tx, item.ID,
			models.ReconciliationOpen, code, &next)
	})
}

// projectionStatusForSourceRequest maps a source request status onto the
// fleet projection vocabulary.
//
// Only terminal states are mapped. An in-flight source status says nothing
// about where fleet's own workflow has reached — a request the source still
// calls "broadcasting" may already be ORGANIZATION_ACCEPTED here — so those
// return empty and ReconcileFromSource leaves the projection status untouched,
// repairing only the version and freshness.
func projectionStatusForSourceRequest(sourceStatus string) string {
	switch sourceStatus {
	case "expired", "no_provider_found":
		return models.ProjectionExpired
	case "cancelled":
		return models.ProjectionCancelled
	default:
		return ""
	}
}

// RunReconciliationLoop drives reconciliation on an interval until ctx ends.
func (s *Service) RunReconciliationLoop(ctx context.Context, interval time.Duration,
	config ReconciliationConfig) error {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := s.RunReconciliation(ctx, config); err != nil && ctx.Err() == nil {
				// One bad pass must not kill the worker; the next tick retries
				// and the cases keep their leases in the meantime.
				continue
			}
		}
	}
}

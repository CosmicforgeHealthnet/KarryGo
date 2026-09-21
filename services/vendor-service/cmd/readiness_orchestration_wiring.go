package main

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"

	"cosmicforge/logistics/services/vendor-service/internal/config"
	"cosmicforge/logistics/services/vendor-service/internal/events"
	readiness "cosmicforge/logistics/services/vendor-service/internal/features/fulfilment_readiness"
	"cosmicforge/logistics/services/vendor-service/internal/features/orchestration"
	staffusecases "cosmicforge/logistics/services/vendor-service/internal/features/staff/usecases"
	"cosmicforge/logistics/services/vendor-service/internal/integrations/logistics"
	"cosmicforge/logistics/services/vendor-service/internal/jobs"
	"cosmicforge/logistics/shared/go/commercemission"
	"cosmicforge/logistics/shared/go/httpx"
	"cosmicforge/logistics/shared/go/logging"
	"cosmicforge/logistics/shared/go/networkanalytics"
	"cosmicforge/logistics/shared/go/serviceauth"
	"cosmicforge/logistics/shared/go/supplypreview"
	"github.com/jackc/pgx/v5"
)

// The composition root for readiness and just-in-time orchestration.
//
// Note the name: this repository already has an internal "Section 11" (the
// cross-authority logistics unification), so this file is named for what it
// wires rather than for a section number that means two different things here.
//
// Every piece is gated by its own flag and every flag defaults off, so a
// deployment that has not certified this section behaves exactly as it did
// before: no routes mounted, no workers running, and the existing synchronous
// fulfilment path untouched.

// staffAuthorizer adapts the staff feature's evaluator to the narrow port the
// two Section 11 handlers need.
//
// It is an adapter rather than a shared interface because the evaluator answers
// a richer question (with a reason) than these surfaces ask, and passing the
// whole evaluator would let a handler start depending on the rest of it.
type staffAuthorizer struct {
	evaluator *staffusecases.AuthorizationEvaluator
}

func (a staffAuthorizer) Authorize(ctx context.Context, staffID, permissionCode string,
	branchID *string) (bool, error) {
	if a.evaluator == nil {
		return false, nil
	}
	decision, err := a.evaluator.Authorize(ctx, staffID, permissionCode,
		staffusecases.Target{BranchID: branchID})
	if err != nil {
		return false, err
	}
	return decision.Allowed, nil
}

// registryGate adapts the logistics registry to the orchestration's gate port.
type registryGate struct{ registry *logistics.Registry }

func (g registryGate) Usable(authority logistics.Authority) bool {
	return g.registry != nil && g.registry.Usable(authority)
}

func (g registryGate) Serviceability(ctx context.Context, authority logistics.Authority,
	requirement logistics.Requirement) (logistics.Serviceability, error) {
	return g.registry.Serviceability(ctx, authority, requirement)
}

func (g registryGate) Capability(ctx context.Context, authority logistics.Authority) (logistics.Capability, error) {
	return g.registry.Capability(ctx, authority)
}

// previewSet adapts the shared supply-preview client set.
//
// An authority this deployment has not configured is simply absent from the
// set, and the preview reports UNKNOWN rather than a refusal -- an authority
// nobody can ask has not said the goods cannot be carried.
type previewSet struct{ clients supplypreview.Set }

func (p previewSet) Preview(ctx context.Context, authority logistics.Authority,
	request supplypreview.Request) (supplypreview.Response, error) {
	client, ok := p.clients.For(string(authority))
	if !ok {
		return supplypreview.Unknown(string(authority), time.Now().UTC(), 0), nil
	}
	return client.Preview(ctx, request)
}

// newSupplyPreviewClients builds one preview client per authority that is both
// enabled and credentialled.
//
// It mirrors newLogisticsCapabilityClients deliberately, including the base
// URLs: the preview hangs off the same internal commerce base as the quote,
// mission and capability paths, so an authority reachable for one is reachable
// for all of them. A disabled authority gets no client, and the preview set
// then reports UNKNOWN for it rather than a refusal -- an authority this
// deployment may not ask has not said the goods cannot be carried.
func newSupplyPreviewClients(cfg config.Config, timeout time.Duration) supplypreview.Set {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	clients := supplypreview.Set{}
	type binding struct {
		authority logistics.Authority
		enabled   bool
		url       string
		secret    string
		apiBase   string
	}
	for _, item := range []binding{
		{logistics.Dispatch, cfg.Flags.DispatchCommerceIntegrationEnabled, cfg.DispatchURL, cfg.DispatchSecret, commercemission.DispatchAPIBase},
		{logistics.Taxi, cfg.Flags.TaxiGoodsDeliveryEnabled, cfg.TaxiURL, cfg.TaxiSecret, commercemission.TaxiAPIBase},
		{logistics.Hauling, cfg.Flags.HaulingCommerceIntegrationEnabled, cfg.HaulingURL, cfg.HaulingSecret, commercemission.HaulingAPIBase},
	} {
		if !item.enabled || item.url == "" || item.secret == "" {
			continue
		}
		clients[string(item.authority)] = supplypreview.New(supplypreview.Config{
			Authority:   string(item.authority),
			BaseURL:     item.url + item.apiBase,
			ServiceName: commercemission.SignatureCaller,
			Secret:      []byte(item.secret),
			Timeout:     timeout,
		})
	}
	return clients
}

// readinessOrchestrationComponents holds what the router and the workers need.
type readinessOrchestrationComponents struct {
	orchestrationService *orchestration.Service
	readinessService     *readiness.Service
	readinessHandler     *readiness.Handler
	orchestrationSvc     *orchestration.Service
	orchestrationRepo    *orchestration.Repository
	orchestrationHandle  *orchestration.Handler

	readinessEnabled     bool
	orchestrationEnabled bool
}

// buildReadinessOrchestration wires readiness and orchestration.
//
// committer is the production mission committer. It is a required argument
// rather than an optional setter, and it is validated below, because a
// deployment that enables just-in-time commitment with no way to commit would
// plan every order and dispatch none of them -- silently, since the
// orchestration would simply park at COMMIT_DUE forever.
func buildReadinessOrchestration(cfg config.Config, pool *pgxpool.Pool,
	evaluator *staffusecases.AuthorizationEvaluator,
	committer orchestration.MissionCommitter) (readinessOrchestrationComponents, error) {

	authorizer := staffAuthorizer{evaluator: evaluator}
	registry := newLogisticsRegistry(cfg, cfg.Fulfilment.QuoteTimeout)
	previews := newSupplyPreviewClients(cfg, cfg.Fulfilment.QuoteTimeout)

	jitEnabled := cfg.Flags.VendorJITPickupEnabled && !cfg.Flags.CommerceGlobalKillSwitch

	// Fail closed.
	//
	// Running with the flag on and no committer is the one configuration that
	// looks healthy and delivers nothing: orders plan, workers evaluate,
	// everything reaches COMMIT_DUE, and no vehicle is ever dispatched. A
	// refused startup is loud; that state is silent.
	if jitEnabled && committer == nil {
		return readinessOrchestrationComponents{}, errors.New(
			"VENDOR_JIT_PICKUP_ENABLED is set but no mission committer could be built; " +
				"just-in-time commitment needs at least one configured logistics authority")
	}
	if jitEnabled && len(previews) == 0 {
		// Warm match is what makes just-in-time timing better than a guess. A
		// deployment with no preview client can still commit, so this is not
		// fatal, but it is worth saying out loud at startup.
		logging.Notice("orchestration",
			"just-in-time commitment is enabled with no supply-preview client; "+
				"commit timing will proceed without warm match")
	}

	readinessRepo := readiness.NewRepository(pool)
	// Section 13: readiness facts go to analytics only when analytics is
	// configured -- without a worker to deliver them they would only
	// accumulate in the outbox.
	var readySignals readiness.ReadySignalSink
	if cfg.AnalyticsURL != "" && cfg.AnalyticsSecret != "" {
		readySignals = readinessAnalyticsSink{outbox: events.NewOutboxRepository(pool)}
	}
	readinessService := readiness.NewService(readiness.Options{
		Repository: readinessRepo,
		Enabled:    cfg.Flags.VendorReadinessOrchestrationEnabled && !cfg.Flags.CommerceGlobalKillSwitch,
		Signals:    readySignals,
	})

	orchestrationRepo := orchestration.NewRepository(pool)
	orchestrationService := orchestration.NewService(orchestration.Options{
		Repository:           orchestrationRepo,
		Gate:                 registryGate{registry: registry},
		Preview:              previewSet{clients: previews},
		Readiness:            readinessService,
		Committer:            committer,
		JITEnabled:           jitEnabled,
		AutoSelectionEnabled: cfg.Flags.VendorAutoAuthoritySelectionEnabled && !cfg.Flags.CommerceGlobalKillSwitch,
	})

	return readinessOrchestrationComponents{
		orchestrationService: orchestrationService,
		readinessService:     readinessService,
		readinessHandler:     readiness.NewHandler(readinessService, authorizer),
		orchestrationSvc:     orchestrationService,
		orchestrationRepo:    orchestrationRepo,
		orchestrationHandle: orchestration.NewHandler(
			orchestrationService, orchestrationRepo, authorizer),
		readinessEnabled:     cfg.Flags.VendorReadinessOrchestrationEnabled && !cfg.Flags.CommerceGlobalKillSwitch,
		orchestrationEnabled: jitEnabled,
	}, nil
}

// registerVendorRoutes mounts the staff-facing surfaces.
func (c readinessOrchestrationComponents) registerVendorRoutes(group *gin.RouterGroup) {
	readiness.RegisterRoutes(group, c.readinessHandler, c.readinessEnabled)
	orchestration.RegisterRoutes(group, c.orchestrationHandle, c.orchestrationEnabled)
}

// orchestrationInternalCallers may ask what Vendor decided.
//
// The three execution authorities are here because each one may legitimately
// need to know whether the work it holds is the work Vendor planned -- a
// reconciling authority that cannot check ends up guessing. Admin and
// analytics are here to explain a decision after the fact.
//
// Nobody else is, and in particular no customer-facing service: which
// authority Vendor chose and when it intends to commit is operational detail,
// and exposing it would make the planner's timing visible to anyone who can
// reach the gateway.
var orchestrationInternalCallers = map[string]bool{
	"driver-dispatch-delivery-service": true,
	"driver-taxi-service":              true,
	"driver-hauling-service":           true,
	"admin-backoffice-service":         true,
	"analytics-service":                true,
}

// registerInternalRoutes mounts the read-only surface other services use.
//
// It is mounted at the engine root rather than under the public vendor base,
// because the gateway forwards that base: a route under it would be one
// misrouted request away from being a public surface, and HMAC signing is the
// only thing standing between this read and anyone on the internet.
func (c readinessOrchestrationComponents) registerInternalRoutes(router *gin.Engine,
	verifier *serviceauth.Verifier) {
	if router == nil || verifier == nil || !c.orchestrationEnabled {
		return
	}
	group := router.Group("/internal/vendor/v1")
	group.Use(serviceauth.Middleware(verifier), allowOrchestrationCaller)
	orchestration.RegisterInternalRoutes(group, c.orchestrationHandle, c.orchestrationEnabled)
}

// allowOrchestrationCaller narrows the signed group to the callers above.
//
// A valid signature proves who is calling, not that they should be. Without
// this, any service holding any Vendor service secret could read every
// planning decision Vendor has made.
func allowOrchestrationCaller(c *gin.Context) {
	raw, _ := c.Get("service_name")
	caller, _ := raw.(string)
	if orchestrationInternalCallers[caller] {
		c.Next()
		return
	}
	c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"success": false, "error": gin.H{
		"code": "forbidden_caller", "message": "This service may not read orchestration decisions.",
		"request_id": httpx.GetRequestID(c)}})
}

// Workers returns the background passes this section needs.
//
// Both run through the same jobs.Job runtime as every other Vendor worker --
// no second scheduler -- and neither is returned at all when just-in-time
// orchestration is off, so a deployment that has not enabled it runs exactly
// the workers it ran before.
//
// They are separate jobs on separate intervals because they answer different
// questions. Evaluation is the hot loop that notices a kitchen finishing
// early; reconciliation chases commits whose outcome was never established,
// and folding it into the evaluation pass would tie how often an unknown
// outcome is chased to how often orders are re-planned.
func (c readinessOrchestrationComponents) Workers() []jobs.Job {
	if !c.orchestrationEnabled || c.orchestrationSvc == nil {
		return nil
	}
	return []jobs.Job{
		{Name: "fulfilment-orchestration-evaluation", Interval: orchestration.EvaluationInterval,
			Pass: func(runCtx context.Context) error {
				_, err := c.orchestrationSvc.RunEvaluationPass(runCtx)
				return err
			}},
		{Name: "fulfilment-orchestration-reconciliation", Interval: orchestration.ReconciliationInterval,
			Pass: func(runCtx context.Context) error {
				_, err := c.orchestrationSvc.RunReconciliationPass(runCtx)
				return err
			}},
		// The stale-signal sweep. It is separate from evaluation because it
		// answers a different question: evaluation asks what is due, this asks
		// what has gone quiet. An order nobody has touched in a quarter of an
		// hour is not due for anything, which is exactly why the evaluation
		// pass would never look at it.
		{Name: "fulfilment-readiness-stale-sweep", Interval: orchestration.StaleSweepInterval,
			Pass: func(runCtx context.Context) error {
				_, err := c.orchestrationSvc.SweepStale(runCtx,
					time.Now().UTC().Add(-orchestration.StaleSweepLookback))
				return err
			}},
	}
}

// readinessAnalyticsSink writes a readiness fact to Vendor's outbox inside the
// readiness transaction (Section 13).
type readinessAnalyticsSink struct{ outbox *events.OutboxRepository }

func (s readinessAnalyticsSink) RecordReady(ctx context.Context, tx pgx.Tx, signal readiness.ReadySignal) error {
	envelope, err := events.NewEnvelope(networkanalytics.EventBranchReadiness, 1, "vendor_order",
		signal.VendorOrderID, signal.Version, signal.BusinessID, "", "", map[string]any{
			"business_id": signal.BusinessID, "branch_id": signal.BranchID, "on_time": signal.OnTime,
			"lateness_seconds": signal.LatenessSeconds, "prep_seconds": signal.PrepSeconds,
		}, false)
	if err != nil {
		return err
	}
	return s.outbox.Append(ctx, tx, envelope)
}
